
FROM debian AS colimafun
#COPY --chown=0 etc/ssh /etc/ssh
#COPY root/.ssh/authorized_keys /root/.ssh/authorized_keys
# colcrt in bsdextrautils
RUN \
  apt-get -y update && \
  apt-get -y install clang curl less libxcb1-dev neovim silversearcher-ag xz-utils \
    mercurial git openssh-server make \
    net-tools \
    screen mosh locales vile bsdextrautils \
    gcc sudo && \
  curl -LO https://go.dev/dl/go1.20.5.linux-arm64.tar.gz && \
  rm -rf /usr/local/go && \
  tar -C /usr/local -xzf go1.20.5.linux-arm64.tar.gz && \
  rm go1.20.5.linux-arm64.tar.gz
COPY bashrc /root/
RUN \
  locale-gen C.UTF-8 && \
  update-locale LANG=C.UTF-8 && \
  useradd -m -d /work -s /bin/bash -u 503 work && \
  chmod 711 /root && \
  cat /root/bashrc >> /root/.profile && \
  cat /root/bashrc >> /work/.profile && \
  rm /root/bashrc && \
  mkdir -p /root/.config/nvim && \
  echo 'source ~/.nvimrc' > /root/.config/nvim/init.vim && \
  echo 'inoremap jk <esc>' > /root/.nvimrc
#COPY --chown=0 --chmod=444 root/.ssh/authorized_keys /root/.ssh/authorized_keys
RUN sed -i 's/^\(%sudo.*\)/\1\nwork ALL=(ALL:ALL) NOPASSWD:ALL/' /etc/sudoers
SHELL ["/bin/bash", "-o", "vi", "-c"]
#CMD mkdir -p /run/sshd && chown work /work && /usr/sbin/sshd -D
USER work
WORKDIR /work
CMD /bin/bash -o vi
