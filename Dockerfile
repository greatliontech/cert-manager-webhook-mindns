FROM golang:1.25-alpine AS build

WORKDIR /workspace

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o webhook -ldflags '-w -s -extldflags "-static"' .

FROM gcr.io/distroless/static:nonroot

COPY --from=build /workspace/webhook /usr/local/bin/webhook

USER nonroot:nonroot

ENTRYPOINT ["webhook"]
