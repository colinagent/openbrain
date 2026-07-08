export type MarkdownPdfExportRemoteSession = {
  hostLabel: string;
  localPort: number;
  remotePort: number;
  wsUrl: string;
  httpUrl: string;
  remoteHome: string;
  workspaceDir: string;
  installDir: string;
};

export type MarkdownPdfExportPayload = {
  title: string;
  content: string;
  sourcePath?: string;
  currentDir?: string;
  remoteSession?: MarkdownPdfExportRemoteSession | null;
  baseDir?: string;
  workspaceRootDir?: string;
  agentsRootDir?: string;
  instanceID?: string;
};
