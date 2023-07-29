FROM golang:1.20 as build

WORKDIR /go/bin

COPY go.mod go.sum ./
RUN go mod download && go mod verify

COPY . .
RUN --mount=type=cache,id=gomod,target=/go/pkg/mod \
    --mount=type=cache,id=gobuild,target=/root/.cache/go-build \
    GOOS=linux CGO_ENABLED=0 go build -v -o /go/bin/app ./...


FROM gcr.io/distroless/static-debian11
COPY --from=build /go/bin/app /
CMD ["/app"]
