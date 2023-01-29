
FROM alpine as pleroma-base
RUN \
( awk 'NR==2' /etc/apk/repositories | sed 's/main/community/' | tee -a /etc/apk/repositories ) && \
apk update && \
apk add curl unzip ncurses postgresql postgresql-contrib nginx certbot file-dev \
    imagemagick ffmpeg exiftool \
    git build-base postgresql-dev \
    bash
RUN git clone https://github.com/postgrespro/rum /tmp/rum && \
    cd /tmp/rum && \
    make USE_PGXS=1 && \
    make USE_PGXS=1 install && \
    cd / && \
    rm -r /tmp/rum
RUN adduser --system --shell  /bin/false --uid 101 --home /opt/pleroma pleroma && \
    mkdir /run/postgresql && chown postgres /run/postgresql && \
    mkdir /etc/pleroma && chown pleroma /etc/pleroma && \
    ln -s /data/var/lib/pleroma /var/lib/pleroma
USER pleroma
RUN \
    curl 'https://git.pleroma.social/api/v4/projects/2/jobs/artifacts/stable/download?job=amd64-musl' -o /tmp/pleroma.zip && \
    cd /tmp && \
    mkdir rel && \
    cd rel && \
    unzip ../pleroma.zip && \
    mv release/* /opt/pleroma/ && \
    cd .. && \
    rm -fr /tmp/pleroma.zip rel

FROM pleroma-base AS setup
USER root
RUN \
    mkdir -p /var/lib/postgresql/data && \
    chmod 0700 /var/lib/postgresql/data && \
    chown postgres /var/lib/postgresql/data && \
    su postgres -s /bin/bash -c 'initdb /var/lib/postgresql/data' && \
    echo "host all  all    0.0.0.0/0  md5" >> /var/lib/postgresql/data/pg_hba.conf && \
    echo "listen_addresses='localhost'" >> /var/lib/postgresql/data/postgresql.conf && \
    su postgres -s /bin/bash -c 'pg_ctl -D /var/lib/postgresql/data start' && \
    psql -U postgres -tc "SELECT 1 FROM pg_database WHERE datname = 'main'" | grep -q 1 || psql -U postgres -c "CREATE DATABASE main" && \
    echo psql -U postgres -c "ALTER USER postgres WITH ENCRYPTED PASSWORD '$(dd if=/dev/urandom count=1 bs=21 2> /dev/null | base64)';" && \
    psql -U postgres -c "ALTER USER postgres WITH ENCRYPTED PASSWORD '$(dd if=/dev/urandom count=1 bs=21 2> /dev/null | base64)';"
COPY --chown=0 --chmod=0755 install-pleroma.sh /opt/pleroma
    
SHELL ["/bin/bash", "-c"]
USER root
CMD \
    dd if=/dev/zero of=/swapfile bs=1024k count=2048 && \
    chmod 600 /swapfile && \
    mkswap /swapfile && \
    swapon /swapfile && \
    chown pleroma /data && \
    if ! [ -d /data/var/lib/postgresql ] ; then \
        mkdir -p /data/var/lib/postgresql/data && \
        chmod 0700 /data/var/lib/postgresql/data && \
        chown postgres /data/var/lib/postgresql/data && \
        cp -rp /var/lib/postgresql /data/var/lib/ ; \
    fi && \
    echo now fly ssh console and run /opt/pleroma/install-pleroma.sh && \
    while true ; do sleep 86400 ; done

FROM ubuntu:22.04 AS soapbox-build
ENV DEBIAN_FRONTEND=noninteractive
RUN apt update && apt upgrade && apt -y install git curl build-essential postgresql postgresql-contrib cmake libmagic-dev imagemagick ffmpeg libimage-exiftool-perl nginx certbot unzip libssl-dev automake autoconf libncurses5-dev
RUN useradd -r -s /bin/false -m -d /var/lib/pleroma -U pleroma
USER pleroma
WORKDIR /opt/pleroma
RUN git clone -b develop https://gitlab.com/soapbox-pub/rebased /opt/pleroma
RUN git clone https://github.com/asdf-vm/asdf.git ~/.asdf --branch v0.10.0
RUN echo ". $HOME/.asdf/asdf.sh" >> ~/.bashrc
RUN echo ". $HOME/.asdf/completions/asdf.bash" >> ~/.bashrc
SHELL ["/bin/bash", "-c"]
RUN source /var/lib/pleroma/.asdf/asdf.sh && asdf plugin-add erlang
RUN source /var/lib/pleroma/.asdf/asdf.sh && asdf plugin-add elixir
RUN source /var/lib/pleroma/.asdf/asdf.sh && asdf install
RUN find / -name mix  2> /dev/null || echo goo
RUN cat ~/.bashrc
RUN source /var/lib/pleroma/.asdf/asdf.sh && mix local.hex --force
RUN source /var/lib/pleroma/.asdf/asdf.sh && mix local.rebar --force
RUN source /var/lib/pleroma/.asdf/asdf.sh && mix deps.get
RUN source /var/lib/pleroma/.asdf/asdf.sh && MIX_ENV=prod mix compile
RUN curl -L https://gitlab.com/soapbox-pub/soapbox/-/jobs/artifacts/develop/download?job=build-production -o /tmp/soapbox.zip
RUN unzip -o /tmp/soapbox.zip -d /opt/pleroma/instance && rm /tmp/soapbox.zip
RUN source /var/lib/pleroma/.asdf/asdf.sh && MIX_ENV=prod mix compile


FROM soapbox-build AS soapbox-run
USER root
RUN \
    ln -s /data/var/lib/pleroma/uploads /var/lib/pleroma/ && \
    ln -s /data/var/lib/pleroma/static /var/lib/pleroma/

EXPOSE 4000
SHELL ["/bin/bash", "-c"]
ENV MIX_ENV=prod
RUN sed -i '/en_US.UTF-8/s/^# //g' /etc/locale.gen && \
    locale-gen
ENV LANG en_US.UTF-8  
ENV LANGUAGE en_US:en  
ENV LC_ALL en_US.UTF-8     
COPY --chown=999 --chmod=0400 config.exs /opt/pleroma/config/prod.secret.exs

CMD \
    dd if=/dev/zero of=/swapfile bs=1024k count=2048 && \
    chmod 600 /swapfile && \
    mkswap /swapfile && \
    swapon /swapfile && \
    chown -R pleroma /data/var/lib/pleroma && \
    chown -R postgres /data/var-exp && \
    su postgres -s /bin/bash -c '/usr/lib/postgresql/14/bin/pg_ctl -D /data/var-exp/lib/postgresql/data start' && \
    while true ; do sleep 86400 ; done

#     su pleroma -s /bin/bash -c 'source /var/lib/pleroma/.asdf/asdf.sh && cd /opt/pleroma && /var/lib/pleroma/.asdf/shims/mix phx.server' && \

#
#    su postgres -s /bin/bash -c 'pg_ctl -D /data/var/lib/postgresql/data start' && \
#    su pleroma -s /bin/bash -lc "./bin/pleroma daemon" && \
#    sudo -Hu pleroma bash -i -c 'MIX_ENV=prod mix ecto.migrate' && \
#


FROM pleroma-base AS default
COPY --chown=101 --chmod=0400 config.exs /etc/pleroma/
EXPOSE 4000
SHELL ["/bin/bash", "-c"]
USER root
ENV PATH=/var/lib/pleroma/.asdf/shims:/var/lib/pleroma/.asdf/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
CMD \
    dd if=/dev/zero of=/swapfile bs=1024k count=2048 && \
    chmod 600 /swapfile && \
    mkswap /swapfile && \
    swapon /swapfile && \
    chown -R pleroma /data/var/lib/pleroma && \
    chown -R postgres /data/var/lib/postgresql && \
    su postgres -s /bin/bash -c 'pg_ctl -D /data/var/lib/postgresql/data start' && \
    su pleroma -s /bin/bash -lc "./bin/pleroma daemon" && \
    while true ; do sleep 86400 ; done

