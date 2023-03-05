FROM registry.gitlab.com/veloren/veloren/server-cli:weekly@sha256:04cf5df077b0ce44102b98
COPY bashrc /root/
EXPOSE 14004
EXPOSE 14005
# volume - "./userdata:/opt/userdata"
ENV RUST_LOG debug,common::net=info
RUN apt-get -y update && \
    apt-get -y install clang curl less libxcb1-dev neovim silversearcher-ag xz-utils \
      mercurial git \
      net-tools \
      screen mosh locales vile bsdextrautils && \                                                               
    locale-gen C.UTF-8 && \
    update-locale LANG=C.UTF-8 && \
    chmod 711 /root && \
    cat /root/bashrc >> /root/.profile && \
    rm /root/bashrc && \
    mkdir -p /root/.config/nvim && \
    echo 'source ~/.nvimrc' > /root/.config/nvim/init.vim && \
    echo 'inoremap jk <esc>' > /root/.nvimrc


SHELL ["/bin/bash", "-o", "vi", "-c"]
ENTRYPOINT dd if=/dev/zero of=/swapfile bs=1024k count=2048 && chmod 600 /swapfile && mkswap /swapfile && swapon /swapfile && /opt/veloren-server-cli

