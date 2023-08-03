FROM golang:1.20 as build

WORKDIR /go/pvc-autoscaler

COPY go.mod go.sum /go/pvc-autoscaler/
RUN go mod download && go mod verify

COPY cmd /go/pvc-autoscaler/cmd
COPY internal /go/pvc-autoscaler/internal

RUN --mount=type=cache,id=gomod,target=/go/pkg/mod \
    --mount=type=cache,id=gobuild,target=/root/.cache/go-build \
    GOOS=linux CGO_ENABLED=0 go build -v -o /go/bin/app /go/pvc-autoscaler/cmd


FROM gcr.io/distroless/static-debian11
COPY --from=build /go/bin/app /
CMD ["/app"]
