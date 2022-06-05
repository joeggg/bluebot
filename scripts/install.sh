set -e

GO="/usr/local/go/bin/go"

echo "** Adding any required users **"
if id "bluebot" &>/dev/null; then
    echo "User already exists"
else
    echo "Creating bluebot user"
    if [ ! -d /opt/bluebot ]; then
        sudo mkdir /opt/bluebot
    fi
    sudo useradd -m -d /opt/bluebot bluebot
fi

sudo chown -R bluebot /opt/bluebot
# Tokens
if [ ! -d /etc/bluebot ]; then
    sudo mkdir /etc/bluebot
    sudo touch /etc/bluebot/token.txt
    sudo touch /etc/bluebot/google_key.txt
fi
# Log dir
if [ ! -d /var/log/bluebot ]; then
    sudo mkdir /var/log/bluebot 
fi
# Tracks dir
if [ ! -d /var/lib/bluebot ]; then
    sudo mkdir /var/lib/bluebot 
fi

sudo chown -R bluebot /etc/bluebot /var/log/bluebot /var/lib/bluebot

echo "** Building executable **"
$GO build
echo "Done building"

echo "** Installing service** "
sudo mv scripts/bluebot.service /etc/systemd/system
sudo systemctl daemon-reload
sudo systemctl restart bluebot.service
echo "Successfully installed service"
