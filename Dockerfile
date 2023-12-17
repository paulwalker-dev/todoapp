FROM --platform=$BUILDPLATFORM docker.io/golang AS build
ARG TARGETARCH
ARG TARGETOS
WORKDIR /app
COPY main.go .
COPY go.mod .
COPY go.sum .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build .

FROM scratch
COPY --from=build /app/todoapp /app
COPY index.gohtml /
CMD ["/app"]
