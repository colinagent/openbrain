import { Buffer } from "node:buffer";
import process from "node:process";
import type { Readable, Writable } from "node:stream";
import {
  AbstractMessageReader,
  AbstractMessageWriter,
  createMessageConnection,
  Disposable,
  type Logger,
  type Message,
  type MessageConnection,
  type MessageReader,
  type MessageWriter,
} from "vscode-jsonrpc/node.js";

export interface StdioTransportOptions {
  stdin?: Readable;
  stdout?: Writable;
  logger?: Logger;
}

export class StdioTransport {
  readonly stdin: Readable;
  readonly stdout: Writable;
  readonly logger: Logger | undefined;

  constructor(options: StdioTransportOptions = {}) {
    this.stdin = options.stdin ?? process.stdin;
    this.stdout = options.stdout ?? process.stdout;
    this.logger = options.logger;
  }

  createConnection(logger: Logger | undefined = this.logger): MessageConnection {
    return createMessageConnection(
      new NdjsonMessageReader(this.stdin),
      new NdjsonMessageWriter(this.stdout),
      logger,
    );
  }
}

export class NdjsonMessageReader extends AbstractMessageReader implements MessageReader {
  private buffer = "";
  private listening = false;

  constructor(private readonly readable: Readable) {
    super();
  }

  listen(callback: (data: Message) => void): Disposable {
    if (this.listening) {
      throw new Error("NDJSON message reader is already listening");
    }
    this.listening = true;

    const onData = (chunk: Buffer | string) => {
      this.buffer += Buffer.isBuffer(chunk) ? chunk.toString("utf8") : chunk;
      this.drain(callback);
    };
    const onError = (error: Error) => this.fireError(error);
    const onEnd = () => {
      this.drain(callback, true);
      this.fireClose();
    };

    this.readable.on("data", onData);
    this.readable.on("error", onError);
    this.readable.on("end", onEnd);
    this.readable.on("close", onEnd);

    return Disposable.create(() => {
      this.readable.off("data", onData);
      this.readable.off("error", onError);
      this.readable.off("end", onEnd);
      this.readable.off("close", onEnd);
    });
  }

  private drain(callback: (data: Message) => void, flush = false): void {
    for (;;) {
      const newline = this.buffer.indexOf("\n");
      if (newline < 0) {
        break;
      }
      const line = this.buffer.slice(0, newline).replace(/\r$/, "");
      this.buffer = this.buffer.slice(newline + 1);
      this.emitLine(line, callback);
    }

    if (flush && this.buffer.trim()) {
      const line = this.buffer;
      this.buffer = "";
      this.emitLine(line, callback);
    }
  }

  private emitLine(line: string, callback: (data: Message) => void): void {
    const trimmed = line.trim();
    if (!trimmed) {
      return;
    }
    try {
      const message = JSON.parse(trimmed) as Message;
      callback(message);
    } catch (error) {
      this.fireError(error);
    }
  }
}

export class NdjsonMessageWriter extends AbstractMessageWriter implements MessageWriter {
  private pending: Promise<void> = Promise.resolve();
  private errorCount = 0;

  constructor(private readonly writable: Writable) {
    super();
    this.writable.on("error", (error) => this.fireError(error));
    this.writable.on("close", () => this.fireClose());
  }

  write(msg: Message): Promise<void> {
    this.pending = this.pending.then(() => this.writeOne(msg));
    return this.pending;
  }

  end(): void {
    if (this.writable !== process.stdout && this.writable !== process.stderr) {
      this.writable.end();
    }
  }

  private writeOne(msg: Message): Promise<void> {
    return new Promise((resolve, reject) => {
      const data = `${JSON.stringify(msg)}\n`;
      const done = (error?: Error | null) => {
        if (error) {
          this.errorCount += 1;
          this.fireError(error, msg, this.errorCount);
          reject(error);
        } else {
          resolve();
        }
      };

      if (this.writable.write(data, "utf8", done)) {
        return;
      }
      this.writable.once("drain", () => resolve());
    });
  }
}
