# Build the loupe binary, then ship it on a tiny base. CGO is off so the binary
# is static and the runtime image needs nothing but the binary.
FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /out/loupe ./cmd/loupe

FROM alpine:3.20
RUN adduser -D -u 10001 loupe
COPY --from=build /out/loupe /usr/local/bin/loupe
USER loupe
ENTRYPOINT ["loupe"]
