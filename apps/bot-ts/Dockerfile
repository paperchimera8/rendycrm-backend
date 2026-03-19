FROM node:20-alpine AS builder
WORKDIR /app

COPY apps/bot-ts/package.json apps/bot-ts/package-lock.json ./
RUN npm ci --no-audit --no-fund

COPY apps/bot-ts/tsconfig.json ./tsconfig.json
COPY apps/bot-ts/src ./src
RUN npm run build

FROM node:20-alpine
WORKDIR /app

COPY apps/bot-ts/package.json ./
COPY --from=builder /app/dist ./dist

ENV PORT=3100
EXPOSE 3100

CMD ["node", "dist/server.js"]
