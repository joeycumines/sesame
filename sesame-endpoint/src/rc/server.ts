import * as net from 'node:net';
import {Code, ConnectError, HandlerContext} from '@connectrpc/connect';
import {create} from '@bufbuild/protobuf';
import {durationMs} from '@bufbuild/protobuf/wkt';
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
} from '../gen/sesame/v1alpha1/tls_pb';
import {ProxyResult} from '../gen/sesame/v1alpha1/proxy_pb';
import {StatusSchema} from '../gen/google/rpc/status_pb';
import {ServerConfig} from '../config';
import {FlowController} from './flowcontrol';
import {
  createNetAddrFromSocket,
  executeProxyHops,
  executeTLSHandshake,
  parseHostPort,
} from './transform';

// Resolves once the socket drains, or rejects if it terminates or the
// request is aborted first. `events.once` alone is not enough: a clean peer
// close emits neither 'drain' nor 'error', so the wait would never settle.
function waitForDrain(socket: net.Socket, signal?: AbortSignal): Promise<void> {
  if (signal?.aborted) {
    return Promise.reject(signal.reason ?? new Error('aborted'));
  }
  return new Promise<void>((resolve, reject) => {
    const cleanup = () => {
      socket.removeListener('drain', onDrain);
      socket.removeListener('close', onClose);
      socket.removeListener('error', onError);
      signal?.removeEventListener('abort', onAbort);
    };
    const onDrain = () => {
      cleanup();
      resolve();
    };
    const onClose = () => {
      cleanup();
      reject(
        new ConnectError(
          'sesame/rc/netconn: upstream socket closed before drain',
          Code.Unavailable,
        ),
      );
    };
    const onError = (err: Error) => {
      cleanup();
      reject(err);
    };
    const onAbort = () => {
      cleanup();
      reject(signal?.reason ?? new Error('aborted'));
    };

    socket.once('drain', onDrain);
    socket.once('close', onClose);
    socket.once('error', onError);
    signal?.addEventListener('abort', onAbort, {once: true});
  });
}

// Maps a google.rpc.Status code (canonical gRPC numbering) to the Connect
// Code enum. Both share the gRPC code space; unmapped/out-of-range values
// fall back to Canceled, matching the Go reference's reset semantics where
// a reset is a client-side cancellation.
function codeFromRpcStatus(code: number): Code {
  if (code >= 0 && code <= 16) {
    return code as Code;
  }
  return Code.Canceled;
}

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

      // Check fingerprint preset and cipher suite restriction upfront
      if (dialReq.tls) {
        const preset = dialReq.tls.fingerprintPreset;
        if (
          preset !== FingerprintPreset.FINGERPRINT_PRESET_UNSPECIFIED &&
          preset !== FingerprintPreset.RUNTIME_DEFAULT &&
          !config.supportedPresets.includes(preset)
        ) {
          throw new ConnectError(
            `sesame/rc/netconn: requested fingerprint preset ${preset} is not supported by standard runtime; custom TLSProvider required`,
            Code.FailedPrecondition,
          );
        }
        if (dialReq.tls.cipherSuites && dialReq.tls.cipherSuites.length > 0) {
          throw new ConnectError(
            'sesame/rc/netconn: cipher_suites restriction is not supported by standard runtime; custom TLSProvider required',
            Code.FailedPrecondition,
          );
        }
      }

      // Connect to target. A set, positive per-request dial timeout
      // overrides the configured default (matching Go, where the request
      // timeout feeds net.Dialer.Timeout directly); otherwise the server
      // default applies.
      let activeSocket: net.Socket;
      let proxyResult: ProxyResult | undefined;
      let tlsResult: TLSHandshakeResult | undefined;
      const requestedTimeoutMs =
        dialReq.timeout !== undefined ? durationMs(dialReq.timeout) : 0;
      const effectiveDialTimeoutMs =
        requestedTimeoutMs > 0 ? requestedTimeoutMs : config.dialTimeoutMs;

      if (dialReq.proxy && dialReq.proxy.hops.length > 0) {
        const pRes = await executeProxyHops(
          targetNetwork,
          targetAddr,
          dialReq.proxy,
          effectiveDialTimeoutMs,
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
          }, effectiveDialTimeoutMs);

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
            if (acquired <= 0) {
              // acquirePartial resolves 0 only for max <= 0, which cannot
              // happen here; treat it as no progress rather than spinning.
              throw new Error('sesame/rc/netconn: no flow-control progress');
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

      const onTimeout = () => {
        if (!isClosed) {
          responseQueue.close(
            new ConnectError(
              'sesame/rc/netconn: upstream socket read timeout',
              Code.DeadlineExceeded,
            ),
          );
        }
        cleanupAll();
      };

      const attachListeners = (s: net.Socket) => {
        s.on('data', onData);
        s.setTimeout(config.readTimeoutMs || 0, onTimeout);
        s.once('end', onEnd);
        s.once('error', onError);
      };

      // Must also clear the timeout: its listener closes over cleanupAll,
      // which acts on the current activeSocket. Leaving it armed on a
      // superseded socket would let it tear down a live upgraded connection.
      const detachListeners = (s: net.Socket) => {
        s.setTimeout(0);
        s.removeListener('timeout', onTimeout);
        s.removeListener('data', onData);
        s.removeListener('end', onEnd);
        s.removeListener('error', onError);
      };

      attachListeners(activeSocket);
      // The socket may arrive paused (a proxy handshake that unshifted
      // coalesced early tunnel bytes pauses before pushing them back, so
      // they are re-emitted to this later 'data' listener). Resume so the
      // first flight is delivered; onData re-pauses per chunk for
      // backpressure, and upgrade paths pause explicitly.
      activeSocket.resume();

      // In-stream TLS Upgrade handler
      const handleUpgradeTLS = async (opts?: TLSOptions) => {
        if (!opts) {
          throw new ConnectError(
            'sesame/rc/netconn: upgrade_tls requires options',
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
                let offset = 0;
                while (offset < bytes.length && !isClosed) {
                  let taken = bytes.length - offset;
                  if (inboundFC) {
                    taken = await inboundFC.acquirePartial(taken, abortSignal);
                  }
                  if (taken <= 0) {
                    // acquirePartial resolves 0 only for max <= 0, which
                    // cannot happen here; fail rather than spinning.
                    throw new Error(
                      'sesame/rc/netconn: no flow-control progress',
                    );
                  }
                  const slice = Buffer.from(
                    bytes.subarray(offset, offset + taken),
                  );
                  offset += taken;
                  const flushRequired = !activeSocket.write(slice);
                  if (flushRequired) {
                    await waitForDrain(activeSocket, abortSignal);
                  }
                  if (isClosed) {
                    break;
                  }
                  if (inboundFC) {
                    // The bytes are now owned by the socket, so the inbound
                    // window is free again. Without this the server stalls
                    // permanently once the client's initial window is spent.
                    inboundFC.addCredit(taken);
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
                                  creditBytes: taken,
                                },
                              ),
                            },
                          }),
                        },
                      }),
                    );
                  }
                }
              }
            } else if (req.data.case === 'control') {
              const ctl = req.data.value;
              if (ctl.action.case === 'upgradeTls') {
                // NOTE (upgrade-vs-window): this await intentionally runs
                // the handshake inline. Parking the request for the socket
                // side was attempted and reverted: the parked design broke
                // the STARTTLS e2e tests (the upgrade never fired because
                // nothing drains the park on the normal path). The residual
                // risk is narrow — an upgrade racing an exhausted outbound
                // window can still mutually wait — and fixing it needs an
                // interruptible credit wait on both stacks, not a park.
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
                // Wire-contract parity with the Go reference: a client
                // reset surfaces as a stream error carrying the reason,
                // indistinguishable neither from a normal end nor from a
                // generic failure.
                const reason = ctl.action.value.reason;
                responseQueue.close(
                  new ConnectError(
                    `sesame/rc/netconn: connection reset by client: ${
                      reason?.message ?? ''
                    }`,
                    reason?.code
                      ? codeFromRpcStatus(reason.code)
                      : Code.Canceled,
                  ),
                );
                cleanupAll();
                break;
              }
            }
          }
          if (!isClosed) {
            // Wire-contract parity: request-stream EOF (CloseSend) means
            // the server initiates a FULL close of the proxy target per
            // the remotecontrol.proto termination contract - matching
            // the Go reference. Responses already buffered in
            // responseQueue still drain (AsyncQueue.next drains queued
            // items before honoring close). Genuine half-close-and-drain
            // is expressed with the halfClose control message instead.
            cleanupAll();
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
