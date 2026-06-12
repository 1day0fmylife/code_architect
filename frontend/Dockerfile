FROM oven/bun:1.3.5 AS deps

WORKDIR /app
COPY package.json bun.lock ./
RUN bun install --frozen-lockfile

FROM deps AS build

ARG VITE_HERMES_API_URL=http://localhost:8088
ENV VITE_HERMES_API_URL=${VITE_HERMES_API_URL}
COPY . .
RUN bun run build

FROM nginx:1.29-alpine AS runtime

COPY --from=build /app/dist /usr/share/nginx/html
EXPOSE 80
