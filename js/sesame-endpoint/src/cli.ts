#!/usr/bin/env node
import {parseConfig, formatHelp} from './config';
import {createEndpointServer, type EndpointServer} from './server';
import {getVersion} from './version';

export async function main() {
  let result: ReturnType<typeof parseConfig>;
  try {
    result = parseConfig(process.argv.slice(2), process.env);
  } catch (err: unknown) {
    process.stderr.write(
      `sesame-endpoint: ${err instanceof Error ? err.message : String(err)}\n\n${formatHelp()}\n`,
    );
    process.exit(1);
  }

  if (result.helpRequested) {
    process.stdout.write(formatHelp());
    process.exit(0);
  }

  if (result.versionRequested) {
    let version: string;
    try {
      version = getVersion();
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : String(err);
      process.stderr.write(`sesame-endpoint: ${msg}\n`);
      process.exit(1);
    }
    process.stdout.write(`sesame-endpoint v${version}\n`);
    process.exit(0);
  }

  if (!result.config) {
    process.stderr.write('sesame-endpoint: failed to parse configuration\n');
    process.exit(1);
  }

  // createEndpointServer resolves and validates TLS material eagerly, so a
  // construction failure (e.g. malformed PEM) must surface as the same
  // friendly one-line error as configuration failures, not a raw stack.
  let server: EndpointServer;
  try {
    server = createEndpointServer(result.config);
  } catch (err: unknown) {
    const msg = err instanceof Error ? err.message : String(err);
    process.stderr.write(`sesame-endpoint: ${msg}\n`);
    process.exit(1);
  }

  const shutdown = async (signal: string) => {
    process.stderr.write(`\nReceived ${signal}, shutting down...\n`);
    try {
      await server.close();
      process.exit(0);
    } catch (err) {
      process.stderr.write(`Error during shutdown: ${err}\n`);
      process.exit(1);
    }
  };

  process.on('SIGINT', () => void shutdown('SIGINT'));
  process.on('SIGTERM', () => void shutdown('SIGTERM'));

  try {
    const bound = await server.listen(result.config.port, result.config.host);
    process.stderr.write(
      `sesame-endpoint listening on ${bound.host}:${bound.port}\n`,
    );
  } catch (err: unknown) {
    const msg = err instanceof Error ? err.message : String(err);
    process.stderr.write(`sesame-endpoint failed to start: ${msg}\n`);
    process.exit(1);
  }
}

if (require.main === module) {
  void main();
}
