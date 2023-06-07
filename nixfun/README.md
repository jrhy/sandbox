```
mkdir -p $HOME/.config/nix
cat > $HOME/.config/nix/nix.conf <<EOF
experimental-features = nix-command flakes
EOF
docker run --mount type=bind,source=$HOME/.config/nix,target=/root/.config/nix   -ti nixos/nix  bash -c 'echo hi | nix run "nixpkgs#ponysay"'
```

works but doesn't do any caching. Using "nix build", e.g. in [eg-pony/Dockerfile](eg-pony/Dockerfile] shows caching.

