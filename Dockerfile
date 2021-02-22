

############################################################
# build
############################################################
FROM registry.cn-hangzhou.aliyuncs.com/ossrs/srs:dev AS build

COPY . /tmp/hgsw
RUN cd /tmp/hgsw && make
# Install binary.
RUN cp /tmp/hgsw/objs/http-gif-sls-writer /usr/local/bin/
RUN cp /tmp/hgsw/main.conf /usr/local/etc/main.conf

############################################################
# dist
############################################################
FROM centos:7 AS dist

# HTTP/1987
EXPOSE 1987
# SRS binary, config files and srs-console.
COPY --from=build /usr/local/bin/http-gif-sls-writer /usr/local/bin/
COPY --from=build /usr/local/etc/main.conf /usr/local/etc/
# Default workdir and command.
WORKDIR /usr/local
CMD ["./bin/http-gif-sls-writer", \
    "-c", "./etc/main" \
    ]
