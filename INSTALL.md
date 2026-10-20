# Installing ezHealthKonnect

ezHealthKonnect installs natively on Windows and Linux servers. Docker is not used.

The full guide covers installation, the headless answer file, upgrades with
automatic rollback, manual rollback, uninstalling and troubleshooting. It is
in **[docs/INSTALL_GUIDE.md](docs/INSTALL_GUIDE.md)**.

Quick start:

```bash
# Linux (x86_64, systemd)
chmod +x ezHealthKonnect-Setup-Linux-x64
sudo ./ezHealthKonnect-Setup-Linux-x64                                  # setup in a browser
sudo ./ezHealthKonnect-Setup-Linux-x64 install --config answers.json    # headless
```

On Windows, run `ezHealthKonnect-Setup-Win64.exe` as an administrator.

Verify the download against `SHA256SUMS.txt` before running it. When the
install finishes, open the one-time setup link it prints to create the first
administrator.
