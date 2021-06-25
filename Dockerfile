ARG VERSION=develop
ARG GO_VERSION=1.16.4

FROM --platform=${BUILDPLATFORM} golang:${GO_VERSION}-alpine as build

RUN apk --no-cache add make ca-certificates
RUN adduser -D ci
WORKDIR /src
COPY go.mod go.sum /src/
RUN go mod download
COPY . /src/
ARG TARGETOS
ARG TARGETARCH
ARG VERSION
RUN GOOS=${TARGETOS} GOARCH=${TARGETARCH} VERSION=${VERSION} make build
USER ci
ENTRYPOINT [ "/src/bin/lru-ci-registry-proxy" ]

FROM --platform=${TARGETPLATFORM} gcr.io/distroless/static as release

COPY --from=build /etc/passwd /etc/group /etc/
COPY --from=build /src/bin/lru-ci-registry-proxy /bin/lru-ci-registry-proxy
USER ci
ENTRYPOINT [ "/bin/lru-ci-registry-proxy" ]
