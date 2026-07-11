# syntax=docker/dockerfile:1

FROM golang:1.25 AS build
WORKDIR /src

COPY --from=kanaka . /kanaka
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o /out/app ./cmd/app


FROM scratch
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /out/app /app
COPY --from=build /src/testdata/seed /seed
USER 65532:65532
ENTRYPOINT ["/app"]
