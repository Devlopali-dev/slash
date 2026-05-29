# Build frontend dist.
FROM node:22-alpine AS frontend
WORKDIR /frontend-build

COPY . .

WORKDIR /frontend-build/frontend/web

RUN npm install -g pnpm@11 && pnpm i --frozen-lockfile

RUN pnpm build

# Build backend exec file.
FROM golang:1.25-alpine AS backend
WORKDIR /backend-build

COPY . .
COPY --from=frontend /frontend-build/frontend/web/dist /backend-build/server/route/frontend/dist

RUN CGO_ENABLED=0 go build -o slash ./bin/slash/main.go

# Make workspace with above generated files.
FROM alpine:latest AS monolithic
WORKDIR /usr/local/slash

RUN apk add --no-cache tzdata wget
ENV TZ="UTC"

COPY --from=backend /backend-build/slash /usr/local/slash/

EXPOSE 5231

# Directory to store the data, which can be referenced as the mounting point.
RUN mkdir -p /var/opt/slash
VOLUME /var/opt/slash

ENV SLASH_MODE="prod"
ENV SLASH_PORT="5231"

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD wget -qO- http://localhost:5231/healthz || exit 1

ENTRYPOINT ["./slash"]
