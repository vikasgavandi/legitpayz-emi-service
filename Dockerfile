FROM golang:1.24 AS build
ENV GOTOOLCHAIN=auto
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download || true
COPY . .
RUN CGO_ENABLED=0 go build -o /svc .

FROM gcr.io/distroless/static:nonroot
COPY --from=build /svc /svc
ENV PORT=8004
EXPOSE 8004
CMD ["/svc"]
