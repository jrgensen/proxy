FROM busybox

ADD bin/proxy /bin/

EXPOSE 80
CMD proxy -port 80 -cookie_name $COOKIE_NAME -pubkeyfile $JWT_PUBCERT_PATH
