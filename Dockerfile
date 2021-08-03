ARG VERSION=develop
ARG GO_VERSION=1.16.4

FROM --platform=${BUILDPLATFORM} golang:${GO_VERSION}-alpine as build

RUN apk --no-cache add make ca-certificates
WORKDIR /src
COPY go.mod go.sum /src/
RUN go mod download
COPY . /src/
ARG TARGETOS
ARG TARGETARCH
ARG VERSION
RUN GOOS=${TARGETOS} GOARCH=${TARGETARCH} VERSION=${VERSION} make build
ENTRYPOINT [ "/src/bin/dockhand-lru-registry" ]

FROM --platform=${TARGETPLATFORM} gcr.io/distroless/static as release

COPY --from=build /etc/passwd /etc/group /etc/
COPY --from=build /src/bin/dockhand-lru-registry /bin/dockhand-lru-registry
ENTRYPOINT [ "/bin/dockhand-lru-registry" ]
