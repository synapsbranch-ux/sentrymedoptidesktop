# Application bug audit

This audit records reproducible defects found during the mobile/LAN and desktop lifecycle review. “Fixed” means the code path and automated coverage were changed; target-device validation remains part of release acceptance.

| Area | Defect/root cause | Resolution | Status |
|---|---|---|---|
| Mobile navigation | The drawer used a full-height flex column without `min-height: 0`; its flex navigation child could grow instead of becoming scrollable. Global vertical overscroll was also disabled. | Added a bounded `100dvh` drawer, `min-h-0`, touch pan, momentum scrolling and vertical page scrolling. | Fixed |
| Mobile dialogs | Centered fixed dialogs were difficult to operate with a small visual viewport or open keyboard, and nested long forms could hide actions. | Dialogs are bottom sheets on mobile, centered on desktop, capped by `100dvh`, independently scrollable, safe-area aware, with sticky close and action controls. | Fixed |
| Mobile “Try again” | Network failures were collapsed into an opaque generic error, SSE had no fallback, and expired sessions did not consistently return the shell to login. | Added timeout/offline error codes and guidance, automatic session-expiry handling, SSE plus revision-poll fallback, and explicit reconnect actions. | Fixed |
| PWA updates | An activated service worker could leave an older shell controlling an open mobile tab until a later lifecycle transition. | Bumped the shell cache and added `skipWaiting`/`clients.claim` for prompt local-server updates. | Fixed |
| Clinic logo | The logo GET route required authentication. Normal browser image requests cannot attach the Wails in-memory desktop session header, so previews and print headers failed. The shell did not use the configured logo. | Added a read-only public branding asset route, retained doctor-only upload, cache-busted live refresh, and rendered the logo in shell, settings, print and public display. | Fixed |
| Desktop close | All platforms intercepted close and minimized, but only Windows had a native tray menu. On Linux/macOS the hidden server could not be reopened or exited normally. | Minimize-to-tray is now Windows-only. Other platforms perform a bounded graceful HTTP shutdown and exit; forced close follows if live SSE clients exceed the grace period. | Fixed |
| Mobile startup size | Every operational page was bundled into the initial JavaScript file. | Route-level code splitting reduced the initial production bundle and loads feature screens on demand. | Fixed |
| Settings integrity | The generic settings update route accepted arbitrary keys and had no schema validation for appearance/public display. | Added an allow-list, optimistic versions and server validation for supported palettes, modes, radii and privacy options. | Fixed |
| Public display privacy | A public flow screen did not exist; using the authenticated queue endpoint would expose chart identifiers and names. | Added a dedicated minimal read model with generated queue codes and configurable ticket-only/initials/first-name labels. | Fixed |

## Release/device checks still required

- Verify WebKit scrolling and the software keyboard on the exact Fedora/macOS build and clinic Android/iOS devices.
- Verify Windows minimize/restore/exit using the packaged native tray build.
- Verify the chosen logo on the clinic printer’s A4/A5 output.
- Verify LAN reconnect while moving a phone between access points and after device sleep.
- Perform a privacy walk-through before enabling first-name mode on a public screen; queue-number-only is the default.

## Smart-clinic backlog (not presented as implemented)

- optional audible queue chime without speaking patient names;
- room/device pairing with a doctor-controlled “call next” action;
- local kiosk check-in using appointment code or QR, with staff confirmation;
- LAN-only operational telemetry for server disk space, backup age and device connectivity;
- scheduled waiting-room messages and bilingual French/Haitian Creole display copy;
- printer/label station profiles and barcode workflows;
- optional local network UPS/power-loss alerting and automatic safe-shutdown integration.
