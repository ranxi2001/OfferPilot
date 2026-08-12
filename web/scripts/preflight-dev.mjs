import { existsSync } from 'node:fs';
import { createConnection } from 'node:net';
import { resolve } from 'node:path';

const port = Number.parseInt(process.env.PORT ?? '3000', 10);
const lockPath = resolve('.next', 'dev', 'lock');

function isPortInUse(targetPort) {
  return new Promise((resolveCheck) => {
    const socket = createConnection({ host: '127.0.0.1', port: targetPort });
    socket.setTimeout(500);
    socket.once('connect', () => {
      socket.destroy();
      resolveCheck(true);
    });
    socket.once('timeout', () => {
      socket.destroy();
      resolveCheck(false);
    });
    socket.once('error', () => resolveCheck(false));
  });
}

const portInUse = await isPortInUse(port);
const lockExists = existsSync(lockPath);

if (portInUse || lockExists) {
  console.error('\nOfferPilot Web development server cannot start safely.');
  if (portInUse) console.error(`- Port ${port} is already in use.`);
  if (lockExists) console.error(`- Next.js development lock exists: ${lockPath}`);
  console.error('\nStop the existing Next.js process, then run npm run dev again.');
  console.error(`Windows: netstat -ano | findstr :${port}`);
  console.error(`macOS/Linux: lsof -i :${port}`);
  process.exit(1);
}

console.log(`Development preflight passed: port ${port} is free.`);
