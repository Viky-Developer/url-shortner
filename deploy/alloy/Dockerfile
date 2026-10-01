FROM grafana/alloy:v1.4.3

COPY config.alloy /etc/alloy/config.alloy

ENTRYPOINT ["/bin/alloy"]
CMD ["run", "--server.http.listen-addr=0.0.0.0:10000", "--storage.path=/var/lib/alloy/data", "/etc/alloy/config.alloy"]
