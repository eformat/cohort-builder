# cohort-builder — multi-stage: UI build → Go build (CGO off) → UBI minimal runtime
FROM registry.access.redhat.com/ubi9/nodejs-20-minimal:latest AS web
USER root
WORKDIR /src
COPY web/package.json web/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY web/ ./
RUN npm run build

FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/dist internal/server/static
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/cohort-builder ./cmd/server

FROM registry.access.redhat.com/ubi9/ubi-minimal:latest
RUN microdnf install -y ca-certificates && microdnf clean all
COPY --from=build /out/cohort-builder /usr/local/bin/cohort-builder
# non-root: binary listens on 8080 unprivileged; SQLite lives on a mounted volume
USER 1001:0
ENV COHORT_HOME=/app/data
VOLUME /app/data
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/cohort-builder"]
CMD ["--addr", "0.0.0.0:8080", "--db", "/app/data/cohort.db"]
