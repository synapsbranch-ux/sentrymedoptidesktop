// Local-only: the preinstalled Chromium in this container is a different build
// number than the pinned @playwright/test expects. Not committed.
import base from "./playwright.config";
import { defineConfig } from "@playwright/test";

export default defineConfig({
  ...base,
  projects: (base.projects ?? []).map((project) => ({
    ...project,
    use: { ...project.use, launchOptions: { executablePath: "/opt/pw-browsers/chromium" } },
  })),
});
