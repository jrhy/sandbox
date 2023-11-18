
FROM debian AS default
COPY --chown=0 etc/ssh /etc/ssh
COPY bashrc /root/
COPY root/.ssh/authorized_keys /root/.ssh/authorized_keys
# colcrt in bsdextrautils
RUN \
  apt-get -y update && \
  apt-get -y install clang curl less libxcb1-dev neovim silversearcher-ag xz-utils \
    mercurial git openssh-server \
    net-tools \
    screen mosh locales vile bsdextrautils sudo && \
  locale-gen C.UTF-8 && \
  update-locale LANG=C.UTF-8 && \
  useradd -m -d /work -s /bin/bash work && \
  chmod 444 /root/.ssh/authorized_keys && \
  chmod 711 /root && \
  cat /root/bashrc >> /root/.profile && \
  rm /root/bashrc && \
  mkdir -p /root/.config/nvim && \
  echo 'source ~/.nvimrc' > /root/.config/nvim/init.vim && \
  echo 'inoremap jk <esc>' > /root/.nvimrc
COPY --chown=0 --chmod=444 root/.ssh/authorized_keys /root/.ssh/authorized_keys
RUN curl -LO https://go.dev/dl/go1.20.5.linux-amd64.tar.gz && rm -rf /usr/local/go && tar -C /usr/local -xzf go1.20.5.linux-amd64.tar.gz && rm go1.20.5.linux-amd64.tar.gz
#RUN useradd -m -d /devpod -s /bin/bash devpod && \
#  groupmod -a -U devpod sudo && \
#  sed -i 's/^\(%sudo.*\) ALL$/\1 NOPASSWD:ALL/' /etc/sudoers
#COPY --chown=devpod --chmod=444 devpod/.ssh/authorized_keys /devpod/.ssh/authorized_keys
SHELL ["/bin/bash", "-o", "vi", "-c"]
CMD dd if=/dev/zero of=/swapfile bs=1024k count=2048 && chmod 600 /swapfile && mkswap /swapfile && swapon /swapfile && mkdir -p /run/sshd && chown work /work && /usr/sbin/sshd -D

