# binary

Every release attaches a static `stashito` binary for `linux/amd64`,
`linux/arm64`, `linux/arm/v7`, `linux/arm/v6` and `linux/386`, plus
`checksums.txt` with the sha256 of every release file.

## download and verify

```sh
ARCH=amd64
BASE=https://github.com/RCM7/stashito/releases/latest/download
curl -fLO ${BASE}/stashito_linux_${ARCH}.tar.gz
curl -fLO ${BASE}/checksums.txt
sha256sum -c --ignore-missing checksums.txt
tar -xzf stashito_linux_${ARCH}.tar.gz stashito
sudo install stashito /usr/local/bin/
```

`ARCH` is `amd64`, `arm64`, `armv7`, `armv6` or `386`. A Raspberry Pi on
64-bit Raspberry Pi OS is `arm64`, on 32-bit it is `armv7`. For a pinned
release, swap `latest/download` for `download/vX.Y.Z`.

## run with systemd

```ini
# /etc/stashito.env
PORT=8080
STORAGE_PATH=/var/lib/stashito
LOG_LEVEL=info
LOG_FORMAT=text
TAG_TTL=60s
UPSTREAM_DOCKERHUB_HOST=registry-1.docker.io
```

```ini
# /etc/systemd/system/stashito.service
[Unit]
Description=Stashito
After=network-online.target
Wants=network-online.target

[Service]
EnvironmentFile=/etc/stashito.env
ExecStart=/usr/local/bin/stashito
DynamicUser=yes
StateDirectory=stashito
Restart=on-failure

[Install]
WantedBy=multi-user.target
```

```sh
sudo systemctl daemon-reload
sudo systemctl enable --now stashito
curl http://localhost:8080/healthz
```

`StateDirectory=stashito` creates `/var/lib/stashito` owned by the service
user. Every variable from [configuration](/configuration) goes in the env
file.
