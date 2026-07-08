import net from 'node:net';
import type { ClientChannel } from 'ssh2';
import type { SshHostWithSecrets } from '../ssh/sshTypes';
import { connectSshClient } from './ssh2Connection';

export type SshForward = {
  close: () => void;
};

export async function runSsh(host: SshHostWithSecrets, command: string, timeoutMs = 120_000) {
  const client = await connectSshClient(host);
  return new Promise<{ stdout: string; stderr: string }>((resolve, reject) => {
    let stdout = '';
    let stderr = '';
    let settled = false;
    let stream: ClientChannel | null = null;

    const cleanup = () => {
      clearTimeout(timer);
      client.end();
    };

    const finish = (error?: Error, code = 0) => {
      if (settled) {
        return;
      }
      settled = true;
      cleanup();
      if (error) {
        reject(error);
        return;
      }
      if (code !== 0) {
        reject(new Error(`ssh failed: command exited with ${code}\n${stderr || ''}`));
        return;
      }
      resolve({ stdout, stderr });
    };

    const timer = setTimeout(() => {
      stream?.close();
      finish(new Error(`ssh command timed out after ${timeoutMs}ms`));
    }, timeoutMs);

    client.exec(command, (error, nextStream) => {
      if (error) {
        finish(error);
        return;
      }
      stream = nextStream;
      nextStream.on('data', (chunk: Buffer) => {
        stdout += chunk.toString();
      });
      nextStream.stderr.on('data', (chunk: Buffer) => {
        stderr += chunk.toString();
      });
      nextStream.on('error', (streamError: Error) => finish(streamError));
      nextStream.on('close', (code: number | null) => finish(undefined, code ?? 0));
    });
  });
}

export async function startPortForward(
  host: SshHostWithSecrets,
  localPort: number,
  remotePort: number,
  onExit?: () => void,
): Promise<SshForward> {
  const client = await connectSshClient(host);
  const sockets = new Set<net.Socket>();
  const channels = new Set<ClientChannel>();
  let closed = false;
  let exitNotified = false;

  const server = net.createServer((socket) => {
    sockets.add(socket);
    let channel: ClientChannel | null = null;

    const cleanup = () => {
      sockets.delete(socket);
      if (channel) {
        channels.delete(channel);
      }
    };

    socket.once('close', cleanup);
    socket.once('error', cleanup);

    client.forwardOut('127.0.0.1', 0, '127.0.0.1', remotePort, (error, nextChannel) => {
      if (error) {
        socket.destroy(error);
        return;
      }
      channel = nextChannel;
      channels.add(nextChannel);
      nextChannel.once('close', cleanup);
      nextChannel.once('error', cleanup);
      socket.pipe(nextChannel).pipe(socket);
    });
  });

  const close = () => {
    if (closed) {
      return;
    }
    closed = true;
    for (const socket of sockets) {
      socket.destroy();
    }
    for (const channel of channels) {
      channel.close();
    }
    server.close();
    client.end();
  };

  const handleSshClose = () => {
    close();
    if (!exitNotified) {
      exitNotified = true;
      onExit?.();
    }
  };

  client.once('close', handleSshClose);
  client.once('error', handleSshClose);

  await new Promise<void>((resolve, reject) => {
    const onError = (error: Error) => {
      server.off('listening', onListening);
      close();
      reject(error);
    };
    const onListening = () => {
      server.off('error', onError);
      resolve();
    };
    server.once('error', onError);
    server.once('listening', onListening);
    server.listen(localPort, '127.0.0.1');
  });

  return { close };
}
