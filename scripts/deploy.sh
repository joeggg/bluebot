set -e

# Assumes token already there and you can scp to /opt/bluebot
TARGET="$1"
FOLDER="bluebot-copy"
FILES="command/ config/ data/ scripts/ util/ main.go go.mod go.sum"

ssh $TARGET "mkdir $FOLDER"
scp -r $FILES $TARGET:~/$FOLDER
# Todo: cross compile and deploy binary?
ssh $TARGET "pushd $FOLDER; chmod +x scripts/install.sh; scripts/install.sh; popd; rm -rf $FOLDER"
echo "Finished deploy"
