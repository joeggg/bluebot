# Assumes token already there and you can scp to /opt/bluebot
INSTALL_DIR="/opt/bluebot"
FILES="data/ command/ config/ scripts/bluebot.service util/ main.go go.mod go.sum"
GO="/usr/local/go/bin/go"

scp -r $FILES $TARGET:$INSTALL_DIR
# Todo: cross compile and deploy binary?
ssh $TARGET "cd $INSTALL_DIR; echo \"Building executable\"; $GO build;"
echo "Done building"

echo "Installing service"
ssh $TARGET "sudo mv $INSTALL_DIR/bluebot.service /etc/systemd/system;"\
            "sudo systemctl daemon-reload; sudo systemctl start bluebot.service"
echo "Successfully installed service"
