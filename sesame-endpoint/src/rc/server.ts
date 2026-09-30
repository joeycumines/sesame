import * as net from 'node:net';
import {Code, ConnectError, HandlerContext} from '@connectrpc/connect';
import {create} from '@bufbuild/protobuf';
import {
  NetConnRequest,
  NetConnResponse,
  NetConnResponseSchema,
  NetConnResponse_ConnSchema,
  NetConnResponse_CapabilitiesSchema,
  NetConnResponse_ControlSchema,
  NetConnResponse_Control_TLSUpgradedSchema,
  NetConnResponse_Control_TLSUpgradeFailedSchema,
  NetConnResponse_Control_WindowUpdateSchema,
  NetConnResponse_Control_HalfCloseSchema,
  NetConnResponse_Control_PongSchema,
} from '../gen/sesame/v1alpha1/remotecontrol_pb';
import {
  FingerprintPreset,
  TLSHandshakeResult,
  TLSOptions,
} from '../gen/sesame/type/tls_pb';
import {ProxyResult} from '../gen/sesame/type/proxy_pb';
import {StatusSchema} from '../gen/google/rpc/status_pb';
import {ServerConfig} from '../config';
import {FlowController} from './flowcontrol';
import {
  createNetAddrFromSocket,
  executeProxyHops,
  executeTLSHandshake,
  parseHostPort,
} from './transform';

class AsyncQueue<T> {
  private queue: T[] = [];
  private waiters: Array<{
    resolve: (item: IteratorResult<T>) => void;
    reject: (err: Error) => void;
  }> = [];
  private closed = false;
  private closeError?: Error;

  public push(item: T): void {
    if (this.closed) return;
    if (this.waiters.length > 0) {
      const waiter = this.waiters.shift()!;
      waiter.resolve({value: item, done: false});
    } else {
      this.queue.push(item);
    }
  }

  public close(err?: Error): void {
    if (this.closed) return;
    this.closed = true;
    this.closeError = err;
    while (this.waiters.length > 0) {
      const waiter = this.waiters.shift()!;
      if (err) {
        waiter.reject(err);
      } else {
        waiter.resolve({value: undefined as unknown as T, done: true});
      }
    }
  }

  public async next(): Promise<IteratorResult<T>> {
    if (this.queue.length > 0) {
      return {value: this.queue.shift()!, done: false};
    }
    if (this.closed) {
      if (this.closeError) {
        throw this.closeError;
      }
      return {value: undefined as unknown as T, done: true};
    }
    return new Promise<IteratorResult<T>>((resolve, reject) => {
      this.waiters.push({resolve, reject});
    });
  }

  public [Symbol.asyncIterator]() {
    return this;
  }
}

export function createRemoteControlService(config: ServerConfig) {
  return {
    async *netConn(
      reqStream: AsyncIterable<NetConnRequest>,
      context: HandlerContext,
    ): AsyncIterable<NetConnResponse> {
      const abortSignal = context.signal;
      const reqIterator = reqStream[Symbol.asyncIterator]();

      // 1. Initial dial message
      const first = await reqIterator.next();
      if (first.done || !first.value || first.value.data.case !== 'dial') {
        throw new ConnectError(
          'sesame/rc/netconn: expected dial request as first message',
          Code.InvalidArgument,
        );
      }

      const dialReq = first.value.data.value;
      if (!dialReq.address?.address) {
        throw new ConnectError(
          'sesame/rc/netconn: missing target address in dial request',
          Code.InvalidArgument,
        );
      }

      const targetAddr = dialReq.address.address;
      const targetNetwork = dialReq.address.network || 'tcp';

      if (!config.allowedNetworks.includes(targetNetwork)) {
        throw new ConnectError(
          `sesame/rc/netconn: network ${targetNetwork} not allowed by server policy`,
          Code.PermissionDenied,
        );
      }

      // Check fingerprint preset upfront
      if (dialReq.tls) {
        const preset = dialReq.tls.fingerprintPreset;
        if (
          preset !== FingerprintPreset.FINGERPRINT_PRESET_UNSPECIFIED &&
          preset !== FingerprintPreset.RUNTIME_DEFAULT
        ) {
          throw new ConnectError(
            `sesame/rc/netconn: requested fingerprint preset ${preset} is not supported by standard runtime; custom TLSProvider required`,
            Code.FailedPrecondition,
          );
        }
      }

      // Connect to target
      let activeSocket: net.Socket;
      let proxyResult: ProxyResult | undefined;
      let tlsResult: TLSHandshakeResult | undefined;

      if (dialReq.proxy && dialReq.proxy.hops.length > 0) {
        const pRes = await executeProxyHops(
          targetNetwork,
          targetAddr,
          dialReq.proxy,
          config.dialTimeoutMs,
          config.secrets,
        );
        activeSocket = pRes.socket;
        proxyResult = pRes.result;
      } else {
        if (abortSignal?.aborted) {
          throw new ConnectError(
            'sesame/rc/netconn: connection aborted by client',
            Code.Canceled,
          );
        }
        const hp = parseHostPort(targetAddr);
        activeSocket = await new Promise<net.Socket>((resolve, reject) => {
          let resolved = false;
          const s = net.createConnection({host: hp.host, port: hp.port});
          const timer = setTimeout(() => {
            if (resolved) return;
            resolved = true;
            s.destroy();
            reject(
              new ConnectError(
                `sesame/rc/netconn: dial timeout to ${hp.host}:${hp.port}`,
                Code.DeadlineExceeded,
              ),
            );
          }, config.dialTimeoutMs);

          const onAbort = () => {
            if (resolved) return;
            resolved = true;
            clearTimeout(timer);
            s.destroy();
            reject(
              new ConnectError(
                'sesame/rc/netconn: connection aborted by client',
                Code.Canceled,
              ),
            );
          };

          if (abortSignal) {
            abortSignal.addEventListener('abort', onAbort, {once: true});
          }

          s.once('connect', () => {
            if (resolved) return;
            resolved = true;
            clearTimeout(timer);
            if (abortSignal) {
              abortSignal.removeEventListener('abort', onAbort);
            }
            resolve(s);
          });
          s.once('error', (err: Error) => {
            if (resolved) return;
            resolved = true;
            clearTimeout(timer);
            if (abortSignal) {
              abortSignal.removeEventListener('abort', onAbort);
            }
            reject(
              new ConnectError(
                `sesame/rc/netconn: dial error to ${hp.host}:${hp.port}: ${err.message}`,
                Code.Unavailable,
              ),
            );
          });
        });
      }

      if (dialReq.tls) {
        const tRes = await executeTLSHandshake(
          activeSocket,
          dialReq.tls,
          targetAddr,
          config.secrets,
        );
        activeSocket = tRes.tlsSocket;
        tlsResult = tRes.result;
      }

      const serverCaps = create(NetConnResponse_CapabilitiesSchema, {
        supportsFlowControl: config.enableFlowControl,
        supportsOpportunisticTls: config.enableOpportunisticTls,
        maxChunkSize: config.maxChunkSize,
        initialWindowSize: config.initialWindowSize,
        supportedPresets: [...config.supportedPresets],
      });

      // 2. Yield initial Conn response
      yield create(NetConnResponseSchema, {
        data: {
          case: 'conn',
          value: create(NetConnResponse_ConnSchema, {
            local: createNetAddrFromSocket(activeSocket, 'local'),
            remote: createNetAddrFromSocket(activeSocket, 'remote'),
            tls: tlsResult,
            proxy: proxyResult,
            capabilities: serverCaps,
            customAttributes: [],
          }),
        },
      });

      // 3. Bidirectional streaming & demultiplexing
      const clientCaps = dialReq.capabilities;
      const isControlCapable = !!clientCaps;
      const responseQueue = new AsyncQueue<NetConnResponse>();

      let outboundFC: FlowController | undefined;
      let inboundFC: FlowController | undefined;

      if (
        isControlCapable &&
        serverCaps.supportsFlowControl &&
        clientCaps.supportsFlowControl
      ) {
        const clientInitialWin =
          clientCaps.initialWindowSize || config.initialWindowSize;
        const serverInitialWin =
          serverCaps.initialWindowSize || config.initialWindowSize;
        outboundFC = new FlowController(clientInitialWin);
        inboundFC = new FlowController(serverInitialWin);
      }

      let isClosed = false;
      let pausedForUpgrade = false;
      let inFlightProcessing = 0;
      let onIdleResolver: (() => void) | null = null;

      const waitForIdle = (): Promise<void> => {
        if (inFlightProcessing === 0) {
          return Promise.resolve();
        }
        return new Promise<void>(resolve => {
          onIdleResolver = resolve;
        });
      };

      const cleanupAll = () => {
        if (isClosed) return;
        isClosed = true;
        detachListeners(activeSocket);
        activeSocket.destroy();
        outboundFC?.close();
        inboundFC?.close();
        responseQueue.close();
      };

      if (abortSignal) {
        abortSignal.addEventListener('abort', () => cleanupAll(), {once: true});
      }

      // Socket event listeners
      const onData = async (chunk: Buffer) => {
        inFlightProcessing++;
        activeSocket.pause();
        try {
          let offset = 0;
          while (offset < chunk.length && !isClosed) {
            const rem = chunk.length - offset;
            const toTake = Math.min(rem, config.maxChunkSize);
            let acquired = toTake;
            if (outboundFC) {
              acquired = await outboundFC.acquirePartial(toTake, abortSignal);
            }
            const slice = chunk.subarray(offset, offset + acquired);
            offset += acquired;

            responseQueue.push(
              create(NetConnResponseSchema, {
                data: {
                  case: 'bytes',
                  value: new Uint8Array(slice),
                },
              }),
            );
          }
        } catch (err: unknown) {
          if (!isClosed) {
            responseQueue.close(err as Error);
          }
        } finally {
          inFlightProcessing--;
          if (inFlightProcessing === 0 && onIdleResolver) {
            const r = onIdleResolver;
            onIdleResolver = null;
            r();
          }
          if (!pausedForUpgrade && !isClosed) {
            activeSocket.resume();
          }
        }
      };

      const onEnd = () => {
        if (isClosed) return;
        if (
          isControlCapable &&
          (clientCaps.supportsFlowControl ||
            clientCaps.supportsOpportunisticTls)
        ) {
          responseQueue.push(
            create(NetConnResponseSchema, {
              data: {
                case: 'control',
                value: create(NetConnResponse_ControlSchema, {
                  event: {
                    case: 'halfClose',
                    value: create(NetConnResponse_Control_HalfCloseSchema, {}),
                  },
                }),
              },
            }),
          );
        }
        responseQueue.close();
      };

      const onError = (err: Error) => {
        if (!isClosed) {
          responseQueue.close(err);
        }
      };

      const attachListeners = (s: net.Socket) => {
        s.on('data', onData);
        s.once('end', onEnd);
        s.once('error', onError);
      };

      const detachListeners = (s: net.Socket) => {
        s.removeListener('data', onData);
        s.removeListener('end', onEnd);
        s.removeListener('error', onError);
      };

      attachListeners(activeSocket);

      // In-stream TLS Upgrade handler
      const handleUpgradeTLS = async (opts?: TLSOptions) => {
        if (!opts) {
          throw new ConnectError(
            'sesame/rc/netconn: upgrade_tls missing options',
            Code.InvalidArgument,
          );
        }

        pausedForUpgrade = true;
        activeSocket.pause();
        await waitForIdle();
        detachListeners(activeSocket);

        try {
          const {tlsSocket, result} = await executeTLSHandshake(
            activeSocket,
            opts,
            targetAddr,
            config.secrets,
          );
          activeSocket = tlsSocket;
          pausedForUpgrade = false;
          attachListeners(activeSocket);
          activeSocket.resume();

          responseQueue.push(
            create(NetConnResponseSchema, {
              data: {
                case: 'control',
                value: create(NetConnResponse_ControlSchema, {
                  event: {
                    case: 'tlsUpgraded',
                    value: create(NetConnResponse_Control_TLSUpgradedSchema, {
                      result,
                    }),
                  },
                }),
              },
            }),
          );
        } catch (err: unknown) {
          const errCode =
            err instanceof ConnectError ? err.code : Code.Unavailable;
          const message = err instanceof Error ? err.message : String(err);
          responseQueue.push(
            create(NetConnResponseSchema, {
              data: {
                case: 'control',
                value: create(NetConnResponse_ControlSchema, {
                  event: {
                    case: 'tlsUpgradeFailed',
                    value: create(
                      NetConnResponse_Control_TLSUpgradeFailedSchema,
                      {
                        error: create(StatusSchema, {
                          code: errCode,
                          message,
                        }),
                      },
                    ),
                  },
                }),
              },
            }),
          );
          cleanupAll();
          throw err;
        }
      };

      // Request stream reader task
      void (async () => {
        try {
          while (!isClosed) {
            const nextReq = await reqIterator.next();
            if (nextReq.done) break;
            const req = nextReq.value;

            if (req.data.case === 'bytes') {
              const bytes = req.data.value;
              if (bytes.length > 0) {
                await new Promise<void>((resolve, reject) => {
                  activeSocket.write(Buffer.from(bytes), err => {
                    if (err) reject(err);
                    else resolve();
                  });
                });
                if (inboundFC) {
                  responseQueue.push(
                    create(NetConnResponseSchema, {
                      data: {
                        case: 'control',
                        value: create(NetConnResponse_ControlSchema, {
                          event: {
                            case: 'windowUpdate',
                            value: create(
                              NetConnResponse_Control_WindowUpdateSchema,
                              {
                                creditBytes: bytes.length,
                              },
                            ),
                          },
                        }),
                      },
                    }),
                  );
                }
              }
            } else if (req.data.case === 'control') {
              const ctl = req.data.value;
              if (ctl.action.case === 'upgradeTls') {
                await handleUpgradeTLS(ctl.action.value.options);
              } else if (ctl.action.case === 'windowUpdate') {
                outboundFC?.addCredit(ctl.action.value.creditBytes);
              } else if (ctl.action.case === 'halfClose') {
                activeSocket.end();
              } else if (ctl.action.case === 'ping') {
                responseQueue.push(
                  create(NetConnResponseSchema, {
                    data: {
                      case: 'control',
                      value: create(NetConnResponse_ControlSchema, {
                        event: {
                          case: 'pong',
                          value: create(NetConnResponse_Control_PongSchema, {
                            id: ctl.action.value.id,
                            timestampNs: ctl.action.value.timestampNs,
                          }),
                        },
                      }),
                    },
                  }),
                );
              } else if (ctl.action.case === 'reset') {
                cleanupAll();
                break;
              }
            }
          }
          if (!isClosed) {
            activeSocket.end();
          }
        } catch (err: unknown) {
          if (!isClosed) {
            responseQueue.close(err as Error);
          }
        }
      })();

      // Main response generator loop
      try {
        for await (const resp of responseQueue) {
          yield resp;
        }
      } finally {
        cleanupAll();
      }
    },
  };
}
