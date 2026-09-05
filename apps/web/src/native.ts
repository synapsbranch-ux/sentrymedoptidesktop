interface DesktopBridge {
  Exit(): Promise<void>;
  MinimizeServer(): Promise<void>;
  OpenBackupFolder(): Promise<void>;
  OpenMobileAccess(): Promise<void>;
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
