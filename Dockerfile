

############################################################
# build
############################################################
FROM registry.cn-hangzhou.aliyuncs.com/ossrs/srs:dev AS build

RUN make
# Install binary.
RUN cp objs/http-gif-sls-writer /usr/local/bin/http-gif-sls-writer

############################################################
# dist
############################################################
FROM centos:7 AS dist

# HTTP/1987
EXPOSE 1987
# SRS binary, config files and srs-console.
COPY --from=build /usr/local/bin/http-gif-sls-writer /usr/local/bin/
RUN mkdir -p /usr/local/logs
# Default workdir and command.
WORKDIR /usr/local
CMD ["./bin/http-gif-sls-writer", \
    "-port", "1987", "-log", "/usr/local/logs/event.log" \
    ]
