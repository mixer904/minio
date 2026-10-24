# How to enable the `silo` performance profile with tuned

## Prerequisites

Please make sure the following packages are already installed via `dnf` or `apt`: 

- `tuned`
- `curl`

### Install `tuned.conf` performance profile

#### Step 1 - download `tuned.conf` from the referenced link
```
wget https://raw.githubusercontent.com/pgsty/silo/main/docs/tuning/tuned.conf
```

#### Step 2 - install tuned.conf as supported performance profile on all nodes
```
sudo mkdir -p /usr/lib/tuned/silo/
sudo mv tuned.conf /usr/lib/tuned/silo
```

#### Step 3 - enable the Silo performance profile on all nodes
```
sudo tuned-adm profile silo
```
