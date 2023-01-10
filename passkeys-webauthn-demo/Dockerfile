FROM golang AS builder

RUN mkdir /app
WORKDIR /app
ADD go.* /app/
RUN go get  \
	github.com/go-webauthn/webauthn/protocol \
	github.com/go-webauthn/webauthn/webauthn \
	github.com/gorilla/mux \
	github.com/gorilla/securecookie \
	github.com/gorilla/sessions
ADD *.go /app/
RUN CGO_ENABLED=0 go build

FROM scratch AS default
WORKDIR /app
COPY --from=builder /app/server /app
ADD index.html /app/
SHELL ["/app/server"]
CMD ""

