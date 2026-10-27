# packages

Signed apt, dnf and pacman repositories at `https://stashito.com/packages`
carry the latest release. The packages install `/usr/bin/stashito`, a systemd
unit and `/etc/stashito.env` with a working default config that caches
Docker Hub on port 8080. Upgrades come with the normal system upgrade and keep
your env file.

## debian, ubuntu

```sh
sudo install -d /etc/apt/keyrings
sudo curl -fsSLo /etc/apt/keyrings/stashito.asc https://stashito.com/packages/key.asc
echo "deb [signed-by=/etc/apt/keyrings/stashito.asc] https://stashito.com/packages/deb stable main" \
  | sudo tee /etc/apt/sources.list.d/stashito.list
sudo apt update
sudo apt install stashito
```

Architectures: `amd64`, `arm64`, `armhf`, `i386`. Raspberry Pi OS works with
the same lines.

## fedora, rhel

```sh
sudo curl -fsSLo /etc/yum.repos.d/stashito.repo https://stashito.com/packages/rpm/stashito.repo
sudo dnf install stashito
```

dnf asks to import the signing key on first install. Architectures:
`x86_64`, `aarch64`, `armv7hl`, `i386`. Rocky, Alma and CentOS Stream work
with the same lines.

## arch

```sh
curl -fsSL https://stashito.com/packages/key.asc | sudo pacman-key --add -
sudo pacman-key --lsign-key rcm7@fuzik.one
printf '\n[stashito]\nServer = https://stashito.com/packages/arch/$arch\n' | sudo tee -a /etc/pacman.conf
sudo pacman -Sy stashito
```

Architectures: `x86_64`, `aarch64`, `armv7h`, `i686`. Omarchy and other
Arch-based distros work with the same lines.

## start it

```sh
sudo systemctl enable --now stashito
curl http://localhost:8080/healthz
```

Settings live in `/etc/stashito.env`; every variable from
[configuration](/configuration) goes there. Restart after editing:
`sudo systemctl restart stashito`.

## signing key

`https://stashito.com/packages/key.asc`, fingerprint
`0D9D 11DD 3A85 FBD9 4B82  9224 331D 44CE 30C1 5686`. The same key signs the repo metadata, every
`.rpm` and every Arch package. Each release also attaches the packages and a
detached `.sig` per package to its GitHub release, listed in `checksums.txt`.
