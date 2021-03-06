# edit servers config files
sed -i 's/127.0.0.1/0.0.0.0/g' /etc/redis/redis.conf /etc/mysql/my.cnf
echo 'host    all             all             0.0.0.0/32            md5'>>/etc/postgresql/9.4/main/pg_hba.conf
sed -i 's/ulimit/#ulimit/g' /etc/init.d/cassandra
sed  -i 's/AllowAllAuthenticator/PasswordAuthenticator/g' /etc/cassandra/cassandra.yaml
sed  -i 's/AllowAllAuthorizer/CassandraAuthorizer/g' /etc/cassandra/cassandra.yaml

/etc/init.d/mysql start
/etc/init.d/postgresql start
/etc/init.d/redis-server start
/etc/init.d/cassandra start

# create a link to data dir
ln -s /root/code/src/github.com/cgrates/cgrates/data /usr/share/cgrates
# create link to cgrates dir
ln -s /root/code/src/github.com/cgrates/cgrates /root/cgr

#setup mysql
cd /usr/share/cgrates/storage/mysql && ./setup_cgr_db.sh root CGRateS.org

# setup postgres
cd /usr/share/cgrates/storage/postgres && ./setup_cgr_db.sh

# setup cassandra
(sleep 20 && \
        cqlsh -u cassandra -p cassandra -e "alter user cassandra with password 'CGRateS.org';" && \
        cd /usr/share/cgrates/storage/cassandra && ./setup_cgr_db.sh cassandra CGRateS.org && \
        cd /root/cgr)&

#env vars
export GOROOT=/root/go; export GOPATH=/root/code; export PATH=$GOROOT/bin:$GOPATH/bin:$PATH
export GO15VENDOREXPERIMENT=1

# build and install cgrates
cd /root/cgr
#glide -y devel.yaml up
./build.sh

# create cgr-engine link
ln -s /root/code/bin/cgr-engine /usr/bin/cgr-engine

# expand freeswitch conf
cd /usr/share/cgrates/tutorials/fs_evsock/freeswitch/etc/ && tar xzf freeswitch_conf.tar.gz

#cd /root/.oh-my-zsh; git pull

cd /root/cgr
echo "for cgradmin run: cgr-engine -config_dir data/conf/samples/cgradmin"
echo 'export GOROOT=/root/go; export GOPATH=/root/code; export PATH=$GOROOT/bin:$GOPATH/bin:$PATH'>>/root/.zshrc

DISABLE_AUTO_UPDATE="true" zsh
