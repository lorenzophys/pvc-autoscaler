FROM golang:1.20 as build

WORKDIR /go/bin
COPY . .

RUN go mod download
RUN GOOS=linux CGO_ENABLED=0 go build -o /go/bin/app ./...


FROM gcr.io/distroless/static-debian11
COPY --from=build /go/bin/app /
CMD ["/app"]