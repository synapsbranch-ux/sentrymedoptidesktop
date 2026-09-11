interface DesktopBridge {
  Exit(): Promise<void>;
  MinimizeServer(): Promise<void>;
  OpenBackupFolder(path: string): Promise<void>;
  OpenInBrowser(path: string): Promise<void>;
  OpenMobileAccess(): Promise<void>;
  SelectBackupFolder(): Promise<string>;
  SetWindowTitle(title: string): Promise<void>;
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
