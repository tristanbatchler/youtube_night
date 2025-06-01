# Youtube Night

Submit a link to a YouTube video, along with your name. This will be used in a game of "Guess who requested this video?" during the next YouTube Night.

## Setup

### Tools required

#### Node.js (for installing Tailwind)
```
curl -o- https://raw.githubusercontent.com/nvm-sh/nvm/v0.40.3/install.sh | bash # Check the latest version at https://github.com/nvm-sh/nvm?tab=readme-ov-file#installing-and-updating
source ~/.bashrc
nvm install node
```

#### Tailwind
```
npm install tailwindcss@latest
```

#### Air server
```
go install github.com/air-verse/air@latest
```

#### sqlc
```
go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
```

#### Templ
```
go install github.com/a-h/templ/cmd/templ@latest
```

### Packages
```
go mod tidy
```

### Database
Create a PostgreSQL database named `youtube_night`. Run the following SQL commands on your newly created database to setup a user and grant permissions:
```sql
CREATE USER youtube_night WITH PASSWORD 'your_secure_password';
GRANT CONNECT ON DATABASE youtube_night TO youtube_night;
ALTER DATABASE youtube_night OWNER TO youtube_night;
```

### Generate a session key
Copy the output of the following command and use it as your session key in the `.env` file in the next step.
```bash
openssl rand -base64 32
```

## Generate a YouTube API key
You'll have to figure this one out on your own as I'm too lazy to write a guide for it. You can find the documentation here: https://developers.google.com/youtube/v3/getting-started

### Configuration
Create a `.env` file in the `srv` directory with the following content:
```
PG_HOST=<your_postgres_host>
PG_PORT=5432
PG_USER=youtube_night
PG_PASSWORD=<your_secure_password>
PG_DATABASE=youtube_night
WEB_PORT=9000
SESSION_KEY="<your_generated_session_key>"
YT_API_KEY=<your_youtube_api_key>
```

You should replace the placeholders (including the angle brackets) with your actual values. Everything else can probably be left as is, but you are welcome to change them if you know what you are doing.

### Nginx configuration
By default, this is served over HTTP on port 9000. To serve it over HTTPS, you can use Nginx as a reverse proxy. Here is an example configuration:
```nginx
server {
    listen 443 ssl;
    server_name yourdomain.com;

    ssl_certificate /path/to/your/public/certificate.pem;
    ssl_certificate_key /path/to/your/private/key.pem;

    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers HIGH:!aNULL:!MD5;
    ssl_prefer_server_ciphers on;

    # Reverse proxy to ytnight app running on port 9000
    location / {
        proxy_pass http://127.0.0.1:9000;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $remote_addr;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header X-Forwarded-Host $host;
        proxy_set_header X-Forwarded-Server $host;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
    }
}

# HTTP to HTTPS redirect
server {
    listen 80;
    server_name yourdomain.com;
    return 301 https://$host$request_uri;
}
```

## ToDo
 - [x] Add favicons
 - [ ] Abstract emoji/avatar handling/showing
 - [ ] Make stores struct to tidy up dependency injection
 - [ ] Design the page and functionality
 - [ ] Implement the page and functionality
 - [ ] Add tests
 - [ ] Add documentation
 - [ ] Containerize the application
 - [ ] Deploy under a different domain