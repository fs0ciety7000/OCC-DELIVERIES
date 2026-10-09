# syntax=docker/dockerfile:1.7
# OCC DELIVERIES — image unique : binaire PocketBase/Go + SPA React.

# ---------- 1. Frontend ----------
FROM node:22-alpine AS web
WORKDIR /web
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY frontend/ ./
RUN npm run build

# ---------- 2. Backend ----------
FROM golang:1.24-alpine AS api
WORKDIR /src
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/occ .

# ---------- 3. Runtime ----------
FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata wget \
 && adduser -D -H -u 10001 occ \
 && mkdir -p /pb/pb_data && chown -R occ:occ /pb
WORKDIR /pb
COPY --from=api /out/occ /pb/occ
COPY --from=web /web/dist /pb/pb_public
ENV TZ=Europe/Brussels \
    OCC_PUBLIC_DIR=/pb/pb_public
USER occ
EXPOSE 8090
VOLUME ["/pb/pb_data"]
HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 \
  CMD wget -qO- http://127.0.0.1:8090/api/occ/health || exit 1
CMD ["/pb/occ", "serve", "--http=0.0.0.0:8090", "--dir=/pb/pb_data"]
