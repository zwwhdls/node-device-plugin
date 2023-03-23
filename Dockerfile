FROM debian:stretch-slim

COPY . .
RUN cp bin/node-device-plugin /usr/bin/node-device-plugin

CMD ["node-device-plugin"]
