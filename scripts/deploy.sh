# Assumes token already there and you can scp to /opt/bluebot
TARGET="$1"
INSTALL_DIR="/opt/bluebot"
FILES="data/ command/ config/ scripts/ util/ main.go go.mod go.sum"

scp -r $FILES $TARGET:$INSTALL_DIR
# Todo: cross compile and deploy binary?
ssh $TARGET "cd $INSTALL_DIR; sudo chmod 755 scripts/install.sh; scripts/install.sh"
echo "Finished deploy"
