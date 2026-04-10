# syntax=docker/dockerfile:1.6

# ---- Stage 1: build frontend ----
FROM node:20-alpine AS frontend
WORKDIR /app/frontend
COPY frontend/package.json frontend/package-lock.json* ./
RUN npm ci || npm install
COPY frontend/ ./
RUN npm run build

# ---- Stage 2: build backend with embedded frontend ----
FROM golang:1.24-alpine AS backend
WORKDIR /src
RUN apk add --no-cache curl
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
# Replace placeholder with freshly built frontend dist
RUN rm -rf ./web && mkdir -p ./web
COPY --from=frontend /app/frontend/dist/ ./web/
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/bambugo .

# ---- Stage 3: runtime ----
FROM alpine:3.20
RUN apk add --no-cache curl ca-certificates
WORKDIR /app
COPY --from=backend /out/bambugo /app/bambugo
ENV BAMBUGO_CONFIG=/data/config.json \
    BAMBUGO_LOG=/data/debug.log \
    BAMBUGO_GO2RTC_YAML=/shared/go2rtc.yaml
VOLUME ["/data", "/shared"]
EXPOSE 8080
ENTRYPOINT ["/app/bambugo"]
