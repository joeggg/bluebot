GO="/usr/local/go/bin/go"

echo "** Adding any required users **"
if id "bluebot" &>/dev/null; then
    echo "User already exists"
else
    echo "Creating bluebot user"
    # sudo useradd -m -d /opt/bluebot bluebot
    sudo chown -R bluebot /opt/bluebot

    if [ ! -d /etc/bluebot ]; then
        sudo mkdir /etc/bluebot
        sudo touch /etc/bluebot/token.txt
    fi    
    sudo chown -R bluebot /etc/bluebot

    if [ ! -d /var/log/bluebot ]; then
        sudo mkdir /var/log/bluebot 
    fi    
    sudo chown -R bluebot /var/log/bluebot

fi

echo "** Building executable **"
$GO build
echo "Done building"

echo "** Installing service** "
sudo mv scripts/bluebot.service /etc/systemd/system
sudo systemctl daemon-reload
sudo systemctl restart bluebot.service
echo "Successfully installed service"
