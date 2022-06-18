set -e

NAME="bluebot"
GO="/usr/local/go/bin/go"

if [ "$1" == "test" ]; then
    echo "Starting test install"
    LOG_DIR="./log"
    CFG_DIR="./token"
    DATA_DIR="./data"
else
    echo "Starting systemd install"
    INSTALL_DIR="/opt/$NAME"
    LOG_DIR="/var/log/$NAME"
    CFG_DIR="/etc/$NAME"
    DATA_DIR="/var/lib/$NAME"

    echo "** Adding any required users **"
    if id $NAME &>/dev/null; then
        echo "User already exists"
    else
        echo "Creating $NAME user"
        if [ ! -d $INSTALL_DIR ]; then
            sudo mkdir $INSTALL_DIR
        fi
        sudo useradd -m -d $INSTALL_DIR $NAME
    fi

    sudo chown -R $NAME $INSTALL_DIR
fi

echo "** Creating any required folders **"
# Tokens
if [ ! -d $CFG_DIR ]; then
    sudo mkdir $CFG_DIR
    sudo touch $CFG_DIR/token.txt
    sudo touch $CFG_DIR/google_key.txt
fi
# Log dir
if [ ! -d $LOG_DIR ]; then
    sudo mkdir $LOG_DIR
fi
# Data dir
if [ ! -d $DATA_DIR ]; then
    sudo mkdir $DATA_DIR
    sudo mv data/civ_list.scv $DATA_DIR
fi
# Tracks dir
if [ ! -d "$DATA_DIR/tmp" ]; then
    sudo mkdir $DATA_DIR/tmp
fi

echo "** Building executable **"
$GO build
echo "Done building"

if [ "$1" != "test" ]; then
    sudo chown -R $NAME $CFG_DIR $LOG_DIR $DATA_DIR
    echo "** Installing service** "
    sudo mv scripts/bluebot.service /etc/systemd/system
    sudo systemctl daemon-reload
    sudo systemctl restart bluebot.service
    echo "Successfully installed service"
fi
