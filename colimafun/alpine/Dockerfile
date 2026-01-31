
FROM alpine AS alpinecolimafun
RUN \
  apk add clang curl neovim \
    mercurial git openssh-server make \
    net-tools \
    screen mosh \
    gcc doas bash unzip gcompat musl-dev 
RUN  curl -LO https://go.dev/dl/go1.24.7.linux-arm64.tar.gz
RUN  rm -rf /usr/local/go
RUN  tar -C /usr/local -xzf go1.24.7.linux-arm64.tar.gz
RUN  rm go1.24.7.linux-arm64.tar.gz
COPY bashrc /root/
#RUN cd /tmp && curl -LO https://ziglang.org/download/0.9.1/zig-linux-aarch64-0.9.1.tar.xz && ( xzcat zig*xz | tar xf - ) && cd zig*/ && ln -s `pwd`/zig /usr/bin/zig
#RUN cd /tmp && curl -LO https://go.dev/dl/go1.24.0.linux-arm64.tar.gz && tar xvzf go*.tar.gz && ln -s `pwd`/go/bin/go /usr/bin/go
#RUN  locale-gen C.UTF-8 && update-locale LANG=C.UTF-8
RUN adduser --system --shell  /bin/bash --uid 101 --home /work work && \
  adduser work wheel && \
  chmod 711 /root
USER work
WORKDIR /work
RUN ls -actlr
ENV PATH $PATH:/usr/local/go/bin:/work/go/bin
WORKDIR /tmp
USER root
WORKDIR /
RUN apk add readline-dev linux-headers cmake git-lfs
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
  echo 'permit nopass :wheel' > /etc/doas.d/doas.conf && \
  echo 'set editing-mode vi' > /work/.inputrc
USER work
WORKDIR /work
ENV SHELL /bin/bash
CMD ["/bin/bash", "-o", "vi"]
