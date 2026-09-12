# Local deployment and recovery

## Before admitting LAN clients

Run the application under a dedicated standard OS account on a supported, patched server computer. Do not give the application local administrator/domain administrator privileges. Enable full-disk encryption, endpoint protection, automatic security updates and screen locking. Restrict the entire data directory and backup destination to the service account and designated administrators using Windows ACLs or Unix permissions; Go file modes alone do not establish Windows ACLs.

Complete first-run setup in the desktop app or through `https://localhost:8787` on the server, before opening the firewall. Setup requires both a local peer and a loopback Host (or Wails' internal marker). Never run `--seed` against clinic data: seed accounts are for disposable development databases only. Give staff individual accounts and unique passwords; use the nurse role unless doctor privileges are required. Review accounts and revoke access promptly.

Default production listening is `:8787` with automatically generated TLS. Permit inbound traffic only from the clinic devices/subnet in the host firewall. Use `SENTRYMED_ADDRESS=127.0.0.1:8787` for desktop-only use. Production refuses a plaintext LAN bind. Development mode is not a production security configuration.

Put guest Wi-Fi and untrusted devices on a separate network. Do not forward this port from the Internet; do not expose SQLite, SMB data shares, remote desktop or administration ports to patient/guest devices. Remote administration should use a managed VPN and MFA. Keep the live SQLite directory on a local disk, outside sync folders and network shares.

## TLS and mobile devices

Production generates a persistent clinic CA and server certificate under the application data directory. In **System → Mobile & network**, download the CA, verify it against the server with the administrator, and trust it only on authorized clinic devices. Protect the CA private key; do not distribute private keys with the public certificate. Do not train staff to bypass certificate warnings. Review certificates when the hostname/IP changes.

Alternatively, set these before launching the server:

```bash
export SENTRYMED_TLS_CERT=/secure/path/sentrymed.crt
export SENTRYMED_TLS_KEY=/secure/path/sentrymed.key
export SENTRYMED_PUBLIC_URL=https://sentrymed.clinic.local:8787
```

The certificate must cover the chosen name/IP and be trusted by the clients. `SENTRYMED_PUBLIC_URL` must be an HTTPS origin with no credentials, path, query or fragment; never include an explicit `:443` — the app compares `Host` with no port normalization, and a mismatch silently drops the `Secure` cookie flag and HSTS even though the connection is genuinely TLS. Direct TLS is the simplest deployment. For a local TLS reverse proxy, bind the backend to `127.0.0.1:8787`, set `SENTRYMED_AUTO_TLS=false`, configure the exact public HTTPS origin, and preserve the public `Host` header. The app recognizes that origin only from an actual loopback peer; `X-Forwarded-*` headers alone confer no trust. The backend also attributes rate-limiting and audit logging to `X-Forwarded-For`, but only when the peer is loopback — so the proxy must **overwrite** that header with the client it actually observed, never append to whatever the client already sent, or a device could pick its own attribution. Block `/api/v1/setup/complete` at the proxy and finish setup locally first. Do not place the HTTP backend on another network host.

Verify HTTPS, certificate trust, Secure/HttpOnly/SameSite cookies, login/logout, uploads and live updates from a clinic phone before use. Public display should use ticket codes; initials/first names are explicit clinic privacy choices.

## Publicly trusted HTTPS on the clinic LAN (Caddy + Let's Encrypt + DuckDNS)

The self-signed clinic CA above requires installing and trusting it on every
phone by hand. To remove that certificate warning entirely — a normal green
padlock, no per-device trust step — put [Caddy](https://caddyserver.com) in
front of the backend as the local TLS reverse proxy described above, and let
it obtain a real certificate from Let's Encrypt. `deploy/caddy/Caddyfile` is a
ready-to-edit template for this.

Because the clinic server must never be reachable from the Internet, the only
usable ACME challenge is **DNS-01**: Caddy proves domain ownership by writing
a DNS TXT record via a provider API, with no inbound connectivity required at
all. This needs a real public domain name, which is where a free
[DuckDNS](https://www.duckdns.org) subdomain comes in — its public DNS record
is pointed at the server's **private LAN IP** (e.g. `192.168.1.50`). That is a
harmless, unroutable answer from the public Internet, but resolves correctly
for any device physically on the clinic Wi-Fi. Only the DNS-01 challenge
itself ever reaches the Internet; clinic traffic never leaves the LAN.

Runbook, in order:

1. Create a free DuckDNS subdomain (e.g. `sentrymedclinic.duckdns.org`) and
   copy the account token. Use a dedicated DuckDNS account for the clinic —
   the token is account-wide, not scoped to one subdomain, so treat it like a
   password.
2. Give the server a static LAN IP via a router DHCP reservation (e.g.
   `192.168.1.50`); confirm with `ipconfig` after a reboot.
3. On duckdns.org, point the subdomain at that private IP. Verify from
   another Internet-connected machine: `nslookup sentrymedclinic.duckdns.org
   1.1.1.1` must answer with the private IP.
4. **Verify the same lookup resolves correctly from a phone on clinic Wi-Fi**
   (not `NXDOMAIN`). Many routers (Fritz!Box, OpenWRT/dnsmasq,
   pfSense/OPNsense, some ISP routers) filter out a public name that resolves
   to a private address by default ("DNS rebinding protection"). If it fails,
   whitelist the domain in the router's rebind-protection exception list, add
   a router-side DNS override for the name, or point the clinic's
   DHCP-advertised DNS at a resolver that doesn't filter private answers
   (e.g. `1.1.1.1`). Everything below is wasted until this check passes.
5. Get Caddy for Windows with the DuckDNS plugin built in — the prebuilt
   download from caddyserver.com/download (add package "duckdns") needs no Go
   toolchain, or build it yourself with `xcaddy build --with
   github.com/caddy-dns/duckdns`.
6. Confirm nothing already owns ports 80/443 (`netsh http show servicestate`,
   `netstat -ano | findstr ":443 :80"`) — IIS, WinRM, BranchCache and some
   antivirus consoles reserve them via `http.sys`.
7. Place `Caddyfile` (edited with the real subdomain, email and subnet),
   `deploy/caddy/start-caddy.ps1` and `deploy/duckdns/update-duckdns-ip.ps1`
   under `C:\ProgramData\SentryMed\`. Write the DuckDNS token to
   `C:\ProgramData\SentryMed\duckdns-token.txt` and restrict it to the
   account that will run the service (`icacls ... /inheritance:r /grant
   "SYSTEM:(R)"`).
8. Dry-run Caddy interactively against the Let's Encrypt **staging** CA first
   (a commented toggle in the Caddyfile) to prove the DNS-01 flow and token
   without spending production issuance attempts; switch to production
   afterward and clear the staging certificate from local storage.
9. Set these before the backend next starts (machine-level environment
   variables, or the equivalent for however the service is launched):

   ```powershell
   [Environment]::SetEnvironmentVariable("SENTRYMED_ADDRESS",    "127.0.0.1:8787",                      "Machine")
   [Environment]::SetEnvironmentVariable("SENTRYMED_AUTO_TLS",   "false",                                "Machine")
   [Environment]::SetEnvironmentVariable("SENTRYMED_PUBLIC_URL", "https://sentrymedclinic.duckdns.org",  "Machine")
   ```

   Leave `SENTRYMED_TLS_CERT`/`SENTRYMED_TLS_KEY` unset — Caddy holds the
   certificate, the backend must never see it.
10. Complete first-run setup locally (desktop app or `http://127.0.0.1:8787`)
    before the proxy is reachable — both the backend and the Caddyfile above
    require this.
11. Run `scripts/install-caddy-task.ps1` elevated to register the Scheduled
    Tasks and firewall rules, then `Start-ScheduledTask -TaskName "SentryMed
    Caddy"`.
12. Remove any existing Windows Firewall rule for the SentryMed executable or
    port 8787 — the backend is loopback-only now, and a stale LAN-facing rule
    would bypass the TLS Caddy provides.
13. Verify from the server (`curl.exe -sS https://sentrymedclinic.duckdns.org
    /health` returns 200 with no `-k`, and a `POST` to
    `/api/v1/setup/complete` returns 403), then from an actual phone on
    clinic Wi-Fi: no certificate warning; the padlock's issuer reads Let's
    Encrypt; sign in and confirm the newest `sessions`/`audit_logs` row shows
    the phone's real LAN address, not `127.0.0.1`; live queue updates still
    arrive promptly; a document/audio upload still works; **System → Mobile &
    network** shows a green HTTPS badge with no CA-download panel.
14. Re-onboard every device: the origin changed, so cookies, the installed
    PWA and the session all need to be redone once per device. Only then
    remove the old clinic CA from each device's trust store.

This trades the self-signed CA's ability to work fully offline forever for a
dependency on Internet reachability roughly every 60 days, when Caddy renews
the certificate (it needs to reach both Let's Encrypt and the DuckDNS API).
Keep the self-signed path above as the rollback: unset `SENTRYMED_PUBLIC_URL`,
remove `SENTRYMED_AUTO_TLS=false`, restore `SENTRYMED_ADDRESS=:8787`, and stop
Caddy. Also note that the LAN server today is the Wails desktop process
itself — after an unattended reboot, Caddy comes up but the backend does not
respond until someone signs into the clinic desktop; the Caddyfile's
`handle_errors` block says so.

## Thermal receipt printer

Keep the printer paired with the computer that runs the clinic server. Browser
and mobile clients submit authenticated print jobs to that server; do not expose
a separate print service or Bluetooth bridge on the network.

On Fedora, enable Bluetooth, pair/trust the printer as the same standard account
that runs SentryMed, and install the BlueZ command-line utilities used for
discovery (`bluetoothctl` and, where available, `sdptool`). Do not launch the app
with `sudo`. **System → Printers → Search for printers** inspects BlueZ data and
opens the validated RFCOMM destination directly. The installed PT280_6E27 has
been physically confirmed as ESC/POS over Serial Port Profile, RFCOMM channel 1.

Pairing alone is not enough: BlueZ asks a registered agent to authorize the
Serial Port service, and a clinic server has no agent to answer, so an untrusted
printer answers with `PRINTER_PERMISSION_DENIED`. The server repairs this itself
once per job (`bluetoothctl trust` then `connect` on the one configured
address); to do it by hand, run `bluetoothctl` and then `pair 10:22:33:90:6E:27`
(PIN 0000), `trust 10:22:33:90:6E:27`. A `PRINTER_PERMISSION_DENIED` raised
before any connection is attempted means the computer refuses the Bluetooth
socket itself — a sandboxed (Flatpak/Snap) build or an account without access to
the adapter — and no amount of pairing will fix it.

A mobile thermal printer accepts one connection at a time. The server therefore
prints one job at a time, waits for the previous data link to close and retries
briefly when the printer reports `PRINTER_BUSY`, so a double-click or two
clients printing at once no longer fails. A `PRINTER_BUSY` that survives the
retries means something else on the computer holds the printer: check for a
stale binding with `rfcomm show all` and release it with `rfcomm release <dev>`,
or close the other program using it.

On Windows, pair the printer in **Settings → Bluetooth & devices**, open the
device's Bluetooth COM-port properties, note the outgoing port (for example
`COM3`), then save that port under **System → Printers**. The server only accepts
the strict `COM1`–`COM999` form and opens the Windows device path directly.

Print a test page, then a real paid POS receipt. Verify text, 58 mm wrapping,
totals, logo dithering and paper feed on the actual device. If logo bitmap
printing is unsupported, the receipt text still prints. Printer errors are
recorded without patient or medical details and never undo a completed payment.
The selected printer lives in the SQLite settings database and is preserved by
normal upgrades and complete backups.

## Transcription and uploaded documents

Remote settings can enable transcription but can no longer select an executable. An administrator must install a trusted local tool and set `SENTRYMED_TRANSCRIPTION_COMMAND_JSON` to a JSON array of executable plus fixed arguments. For example:

```text
["C:\\ClinicTools\\transcribe.exe","--format","text"]
```

The server appends the saved audio path as the last argument, invokes the executable directly, bounds execution/output, and never evaluates a shell string from settings. Restart after changing the environment. Migration 017 removes the old database command setting; existing installations using transcription must configure this environment variable. Keep the tool and its media parsers patched and run with minimal filesystem/network permissions.

File signature checks, size limits, private responses and confined paths do not make document contents malware-free. The application does not include an antivirus engine. Scan attachments with endpoint protection, disable Office macros and keep PDF/Office viewers patched. Never execute downloaded or emailed software on the clinic server.

## Backups that can survive ransomware

Choose and validate the destination under **System → Backups**. Each new backup consists of:

- `sentrymed-<kind>-<timestamp>.db`: consistent SQLite snapshot, including committed WAL data;
- the matching `.db.files` directory: documents (including recordings), branding, signatures and `manifest.json`;
- the approved database and asset-manifest SHA-256 values from the backup record/API, retained in the administrator's recovery inventory.

Copy the complete pair and record both checksum values. The displayed database size excludes assets: budget for the full directory. These files contain sensitive information and are not application-encrypted. Use encrypted disks/storage, restricted ACLs and a separately protected recovery inventory. The generated CA/private keys, logs and server environment are not in the application backup; manage them separately. A new recovery host should normally receive fresh TLS keys and client trust.

The default automatic policy is every four hours with 30-day retention. Expiry never removes the newest three verified automatic generations. Manual and pre-restore generations are retained; monitor disk capacity and review old generations deliberately. Check both the newest successful backup time and the automatic failure message. Schedule/capacity must fit actual data volume: file snapshots pause short requests while establishing a consistent database/file generation.

A disk or share continuously writable by the clinic server can be encrypted or deleted by malware running as that account. Maintain an additional copy that the server cannot overwrite: a rotated encrypted disk disconnected after copying, or independently administered immutable/offline storage. Prefer an independent backup service that reads completed generations and holds its own credentials. Test its retention lock and restores; merely naming a folder “backup” offers no ransomware protection. Agree on recovery-point and recovery-time targets with clinic management.

## Tested restore drill on an isolated copy

1. Keep the clinic running on its existing server while rehearsing on a separate isolated copy with matching application version. Record patient/invoice counts and representative documents/signatures, plus both approved backup hashes. Restrict any rehearsal using real records to authorized administrators.
2. Ensure enough space for the selected generation, a fresh pre-restore database/file generation, staged replacement files and the retained originals. Disconnect clients during a production restore.
3. In **System → Backups**, select a verified snapshot and use Restore as a doctor. The app validates database integrity, foreign keys and the checksummed file manifest; stages files; creates a pre-restore safety snapshot; then replaces the database/files under maintenance exclusion.
4. Sign in again. Restored sessions are invalidated deliberately. Verify representative demographics/history, appointments, invoice balances, documents, signatures and a recording where present. Check that the pre-restore generation remains available and that a new backup succeeds.
5. Record the result, recovery point and elapsed restore time. Keep at least one previous proven generation. A legacy record with no `assetsChecksumSha256` is database-only and cannot recover lost documents/signatures; create a new complete backup before relying on this procedure.

Automated tests cover document/signature loss followed by recovery, rejected tampering, session invalidation and recovery-catalog retention. They do not establish actual-device recovery time or survival of every storage/power failure. Database and directory renames are not a single crash-atomic filesystem transaction. Use a UPS, stop gracefully and never interrupt restore. On interruption or rollback errors, stop the application and preserve the entire data directory, `restore-assets-*`, `.before-restore-*` and backup generations for administrator recovery; do not repeatedly retry over the preserved originals.

## Recovery after loss or compromise of the server

Disconnect the affected machine from the network, preserve evidence, and rebuild on a clean patched host. Do not restore onto an operating system that may still be compromised. Choose a generation from before the incident and verify its database and manifest hashes against the independently retained recovery inventory, plus every file hash in the manifest. Checksums detect corruption; they do not authenticate a backup if an attacker can replace both data and expected hashes.

With all SentryMed processes stopped, an administrator can place a verified snapshot at `database/sentrymed.db` under a fresh restricted data root and copy its `documents`, `branding` and `signatures` subdirectories from the matching `.db.files` bundle. Do not copy old `-wal` or `-shm` files over the new standalone snapshot. Before reconnecting any clients, invalidate every row in `sessions` using a trusted SQLite administration tool (`UPDATE sessions SET invalidated_at=datetime('now');`), review/disable accounts, rotate credentials and re-establish TLS trust. The in-app restore performs session invalidation automatically; a raw offline file copy does not. Correct the restored backup destination, create a new complete backup, then conduct the validation drill above. Migration 017 runs on first launch of the new application.

If the data root or backup was potentially tampered with, an administrator must review schema/settings and scan documents/tools before reuse. The application cannot attest that an attacker-controlled SQLite file is trustworthy solely from an integrity check.

## Upgrade and Windows release

Stop writes and preserve a complete copy of the current data root before upgrade, especially when upgrading from database-only backups. Deploy the same application version to the desktop/server; refresh browser clients. Migration 017 is forward-only and does not delete existing patient/clinical/financial records. It adds integrity guards and bookkeeping tables, removes the legacy transcription command, and leaves existing scheduling overlaps for staff review. Restore the complete pre-upgrade data root with the previous application version if rollback is necessary; do not selectively drop new tables/triggers on live data.

Build the per-user NSIS installer on Windows with `scripts/build-windows-installer.ps1` or the Windows installer workflow. Sign the executable/installer with the publisher certificate, then test installation, upgrade, tray lifecycle, certificate trust, firewall scope and a complete backup/restore on the target Windows hardware. Cross-compilation is not a substitute for this device acceptance. Do not package production data or passwords.

Closing the Windows desktop window minimizes to tray. Use the explicit Exit action for shutdown. On platforms without that tray implementation, closing the window shuts down the server. Maintain patching, dependency review, access review and periodic recovery drills after release. This application hardening does not guarantee immunity to ransomware or undiscovered vulnerabilities.
