FROM busybox

ADD bin/proxy /bin/

EXPOSE 80
CMD proxy -port 80
