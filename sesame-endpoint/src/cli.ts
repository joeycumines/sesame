#!/usr/bin/env node
import {parseConfig, formatHelp} from './config';
import {createEndpointServer} from './server';

const VERSION = '0.1.0';

async function main() {
  const result = parseConfig(process.argv.slice(2), process.env);

  if (result.helpRequested) {
    process.stdout.write(formatHelp());
    process.exit(0);
  }

  if (result.versionRequested) {
    process.stdout.write(`sesame-endpoint v${VERSION}\n`);
    process.exit(0);
  }

  if (!result.config) {
    process.stderr.write('sesame-endpoint: failed to parse configuration\n');
    process.exit(1);
  }

  const server = createEndpointServer(result.config);

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

void main();
