# Local deployment

## Server computer

Use a dedicated clinic user account on a supported desktop OS. Enable full-disk encryption, OS updates, screen locking and endpoint protection. The application data directory and backup destinations contain medical and financial information; restrict filesystem permissions and physical access.

The default bind address is `:8787`, which permits LAN clients. Use `SENTRYMED_ADDRESS=127.0.0.1:8787` when mobile/browser access is not wanted. Permit inbound traffic only from the private clinic subnet in the host firewall.

## LAN and mobile PWA

Connect devices to the same trusted private Wi-Fi. Open **System → Mobile & network** on desktop and scan the QR code. Do not expose port 8787 to the public Internet or configure consumer-router port forwarding.

PWA installation and service-worker behavior require a secure context; browsers make a localhost exception that does not cover a phone connecting to a LAN IP. SentryMed can terminate TLS itself with a locally trusted certificate:

```bash
export SENTRYMED_TLS_CERT=/secure/path/sentrymed.crt
export SENTRYMED_TLS_KEY=/secure/path/sentrymed.key
export SENTRYMED_PUBLIC_URL=https://sentrymed.clinic.local:8787
```

The certificate must include the chosen DNS name/IP and be trusted by clinic phones. Alternatively, terminate TLS with a locally managed reverse proxy, bind SentryMed to `127.0.0.1:8787`, and set `SENTRYMED_PUBLIC_URL` to the proxy URL. The QR code uses that public URL.

Certificate issuance and trusted local DNS vary by clinic and are intentionally not hard-coded. Automatic local certificate enrollment and mDNS discovery remain deployment features.

## Data and backups

Keep the live SQLite file on the server computer, not in a consumer sync folder. Configure periodic verified snapshots and copy them to a separate encrypted external disk or controlled network location. Regularly test restore on a non-production machine. Retention automatically removes expired **automatic** backups; manual and pre-restore snapshots are retained for deliberate review.

## First launch

Initial setup is accepted only from the server computer. Complete it before enabling inbound firewall access. Use a unique doctor password and create named nurse accounts; do not share logins.

## Shutdown

Closing the desktop window minimizes it and leaves the server active. Use the explicit application exit action before shutting down the server computer so HTTP and SQLite close cleanly. The OS can still recover WAL safely after an unexpected power loss, but an uninterruptible power supply is recommended.
