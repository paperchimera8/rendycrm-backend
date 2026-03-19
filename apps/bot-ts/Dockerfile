FROM node:22-alpine AS build

WORKDIR /app

COPY apps/bot-ts/package.json apps/bot-ts/package-lock.json ./
RUN npm ci

COPY apps/bot-ts/tsconfig.json ./tsconfig.json
COPY apps/bot-ts/src ./src

RUN npm run build

FROM node:22-alpine

WORKDIR /app

COPY --from=build /app/dist ./dist

ENV PORT=3100

EXPOSE 3100

CMD ["node", "dist/server.js"]
