FROM nixos/nix

RUN mkdir -p $HOME/.config/nix \
 && echo experimental-features = nix-command flakes > $HOME/.config/nix/nix.conf
RUN nix build "nixpkgs#ponysay"
RUN echo hi | nix run "nixpkgs#ponysay"
