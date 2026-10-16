FROM alpine:3.22 AS build
RUN apk add --no-cache file
RUN sleep 30 && dd if=/dev/urandom of=/blob bs=1M count=100 2>/dev/null && echo built-blob
COPY input.txt /input.txt
RUN uname -m > /arch

FROM alpine:3.22
COPY --from=build /blob /blob
COPY --from=build /arch /arch
CMD ["cat", "/arch"]
