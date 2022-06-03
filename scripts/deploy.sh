INSTALL_DIR="/opt/bluebot"
FILES="data/ command/ config/ util/ main.go go.mod go.sum"
GO="/usr/local/go/bin/go"

scp -r $FILES $TARGET:$INSTALL_DIR
# Todo: cross compile and deploy binary?
ssh $TARGET "cd $INSTALL_DIR; echo \"Building executable\"; $GO build; exit"
echo "Done!"
