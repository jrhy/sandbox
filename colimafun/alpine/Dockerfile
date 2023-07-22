
FROM alpine AS alpinecolimafun
RUN \
  apk add clang curl neovim \
    mercurial git openssh-server make \
    net-tools \
    screen mosh \
    gcc doas bash unzip gcompat musl-dev 
RUN  curl -LO https://go.dev/dl/go1.20.6.linux-arm64.tar.gz
RUN  rm -rf /usr/local/go
RUN  tar -C /usr/local -xzf go1.20.6.linux-arm64.tar.gz
RUN  rm go1.20.6.linux-arm64.tar.gz
COPY bashrc /root/
RUN cd /tmp && curl -LO https://ziglang.org/download/0.9.1/zig-linux-aarch64-0.9.1.tar.xz && ( xzcat zig*xz | tar xf - ) && cd zig*/ && ln -s `pwd`/zig /usr/bin/zig
RUN cd /tmp && curl -LO https://go.dev/dl/go1.20.6.linux-arm64.tar.gz && tar xvzf go*.tar.gz && ln -s `pwd`/go/bin/go /usr/bin/go
#RUN  locale-gen C.UTF-8 && update-locale LANG=C.UTF-8
COPY  .ssh /work/.ssh
RUN adduser --system --shell  /bin/bash --uid 101 --home /work work && \
  adduser work wheel && \
  chmod 711 /root && \
  chown -R work /work/.ssh && chmod 0400 /work/.ssh/id*
USER work
WORKDIR /work
RUN ls -actlr
RUN \
    echo raspberrypi.local ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIIlc5EM5sp/rXxBv4Yq/cUMp5AsPk6EKr80FQtJDxAIB > /work/.ssh/known_hosts && \
    ssh -i /work/.ssh/id_ed25519 pi@raspberrypi.local echo hi && \
    git clone ssh://pi@raspberrypi.local/~/s3db && cd s3db && git checkout alpine && \
    git config user.email jrhy && \
    git config user.name jrhy
ENV PATH $PATH:/usr/local/go/bin:/work/go/bin
RUN cd s3db && go get -t -v ./...
RUN cd s3db/proto && go generate 
WORKDIR /tmp
RUN curl -LO https://www.sqlite.org/2023/sqlite-amalgamation-3420000.zip && \
	unzip sqlite-amalgamation-3420000.zip
RUN git clone https://github.com/riyaz-ali/sqlite && \
    git clone https://github.com/mattn/go-sqlite3
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
  echo 'permit nopass :wheel' > /etc/doas.d/doas.conf
USER work
WORKDIR /work
ENV SHELL /bin/bash
CMD ["/bin/bash", "-o", "vi"]
