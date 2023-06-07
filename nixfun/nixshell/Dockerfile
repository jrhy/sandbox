FROM nixos/nix AS nixshell

RUN mkdir -p $HOME/.config/nix \
 && echo experimental-features = nix-command flakes > $HOME/.config/nix/nix.conf
ENTRYPOINT ["bash", "-o", "vi"]
