
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
RUN cd /tmp && curl -LO https://ziglang.org/download/0.9.1/zig-linux-aarch64-0.9.1.tar.xz && ( xzcat zig*xz | tar xf - ) && cd zig*/ && ln -s `pwd`/zig /usr/bin/zig
RUN cd /tmp && curl -LO https://go.dev/dl/go1.20.6.linux-arm64.tar.gz && tar xvzf go*.tar.gz && ln -s `pwd`/go/bin/go /usr/bin/go
RUN \
  locale-gen C.UTF-8 && \
  update-locale LANG=C.UTF-8 && \
  useradd -m -d /work -s /bin/bash -u 503 work && \
  chmod 711 /root && \
  apt-get -y install unzip
USER work
WORKDIR /work
RUN git clone https://github.com/jrhy/s3db && cd s3db && go get -t -v ./...
ENV PATH $PATH:/work/go/bin
RUN cd s3db/proto && go generate 
USER root
WORKDIR /
RUN \
  cat /root/bashrc >> /root/.profile && \
  cat /root/bashrc >> /work/.bashrc && \
  rm /root/bashrc && \
  mkdir -p /root/.config/nvim && \
  echo 'source ~/.nvimrc' > /root/.config/nvim/init.vim && \
  echo 'inoremap jk <esc>' > /root/.nvimrc && \
  mkdir -p /work/.config/nvim && \
  echo 'source ~/.nvimrc' > /work/.config/nvim/init.vim && \
  echo 'inoremap jk <esc>' > /work/.nvimrc && \
  sed -i 's/^\(%sudo.*\)/\1\nwork ALL=(ALL:ALL) NOPASSWD:ALL/' /etc/sudoers
USER work
WORKDIR /work
SHELL ["/bin/bash", "-o", "vi", "-c"]
