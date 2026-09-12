interface DesktopBridge {
  Exit(): Promise<void>;
  MinimizeServer(): Promise<void>;
  OpenBackupFolder(path: string): Promise<void>;
  OpenInBrowser(path: string): Promise<void>;
  OpenMobileAccess(): Promise<void>;
  SelectBackupFolder(): Promise<string>;
  SetWindowTitle(title: string): Promise<void>;
  /**
   * Hands `data` to the operating system's own Save As dialog under
   * `suggestedName` and writes it there, returning the chosen path (or ""
   * if the user cancelled). See download.ts — this is the desktop half of
   * every "Download" action in the application; it never goes through the
   * webview's own download handling. `data` is passed as a plain base64
   * string, which Wails' JSON bridge decodes into the Go method's []byte
   * parameter automatically.
   */
  SaveFile(suggestedName: string, base64Data: string): Promise<string>;
}

declare global {
  interface Window {
    go?: {
      main?: {
        DesktopBridge?: DesktopBridge;
      };
    };
  }
}

export const desktopBridge = () => window.go?.main?.DesktopBridge;
