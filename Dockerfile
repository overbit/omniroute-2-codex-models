FROM node:22-alpine
WORKDIR /app
COPY --chown=node:node proxy.js ./proxy.js
USER node
ENV HOST=0.0.0.0 PORT=8787 OMNIROUTE_MODELS_URL=http://host.docker.internal:20128/v1/models?prefix=alias
EXPOSE 8787
CMD ["node", "proxy.js"]
