# OCC DELIVERIES — image unique : binaire PocketBase/Go + SPA React.

# ---------- 1. Frontend ----------
FROM node:24-alpine AS web
WORKDIR /web
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY frontend/ ./
RUN npm run build

# ---------- 2. Backend ----------
FROM golang:1.27-alpine AS api
WORKDIR /src
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
# Coolify transmet SOURCE_COMMIT si « Include Source Commit in Build » est activé.
ARG SOURCE_COMMIT
ARG VERSION=${SOURCE_COMMIT}
RUN CGO_ENABLED=0 go build -trimpath -tags timetzdata -ldflags "-s -w -X main.version=${VERSION:-dev}" -o /out/occ .
RUN CGO_ENABLED=0 go build -trimpath -tags timetzdata -ldflags "-s -w" -o /out/menusync ./cmd/menusync

# ---------- 3. Runtime ----------
FROM alpine:3.24
# ca-certificates et wget (busybox) sont déjà dans alpine ; tzdata est embarqué
# dans le binaire (-tags timetzdata) → aucun accès réseau requis à cette étape.
RUN adduser -D -H -u 10001 occ \
 && mkdir -p /pb/pb_data && chown -R occ:occ /pb
WORKDIR /pb
COPY --from=api /out/occ /pb/occ
COPY --from=api /out/menusync /pb/menusync
COPY --from=web /web/dist /pb/pb_public
ENV TZ=Europe/Brussels \
    OCC_PUBLIC_DIR=/pb/pb_public
USER occ
EXPOSE 8090
VOLUME ["/pb/pb_data"]
HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 \
  CMD wget -qO- http://127.0.0.1:8090/api/occ/health || exit 1
CMD ["/pb/occ", "serve", "--http=0.0.0.0:8090", "--dir=/pb/pb_data"]
