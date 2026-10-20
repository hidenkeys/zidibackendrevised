# Deploying Your WhatsApp Bot on DigitalOcean

This guide will walk you through the process of deploying your Go-based WhatsApp bot on a DigitalOcean droplet.

## Prerequisites

*   A DigitalOcean account.
*   A registered domain name (optional, but recommended for a production environment).
*   A Facebook Developer App with the WhatsApp Business API configured.
*   Your WhatsApp Business API credentials:
    *   `WHATSAPP_PHONE_NUMBER_ID`
    *   `WHATSAPP_ACCESS_TOKEN`
    *   `WHATSAPP_WEBHOOK_VERIFY_TOKEN`

## Step 1: Set Up Your DigitalOcean Droplet

1.  **Create a new droplet:**
    *   Log in to your DigitalOcean account and click the "Create" button, then select "Droplets".
    *   Choose an image: **Ubuntu 22.04 (LTS) x64** is a good choice.
    *   Choose a plan: The "Basic" plan with 1 GB of RAM should be sufficient for this application.
    *   Choose a datacenter region: Select a region that is geographically close to you or your users.
    *   Authentication: Choose "SSH keys" for a more secure setup. If you haven't already, you can follow [this guide](https://www.digitalocean.com/docs/droplets/how-to/add-ssh-keys/create-with-openssh/) to create and add an SSH key to your account.
    *   Choose a hostname for your droplet (e.g., `whatsapp-bot`).
    *   Click the "Create Droplet" button.

2.  **Connect to your droplet:**
    *   Once your droplet is created, you'll see its IP address on your DigitalOcean dashboard.
    *   Open a terminal and connect to your droplet using SSH:
        ```bash
        ssh root@<your_droplet_ip>
        ```

## Step 2: Configure Your Domain Name (Optional)

If you have a domain name, you can point it to your droplet's IP address. This will allow you to use a custom domain for your webhook URL.

1.  **Create an A record:**
    *   In your domain registrar's DNS settings, create a new "A" record.
    *   Set the "Host" or "Name" to `@` (for the root domain) or a subdomain (e.g., `whatsapp`).
    *   Set the "Value" or "Points to" to your droplet's IP address.

## Step 3: Install and Configure Nginx

Nginx will act as a reverse proxy for your Go application. It will handle incoming HTTP and HTTPS requests and forward them to your application.

1.  **Install Nginx:**
    ```bash
    sudo apt update
    sudo apt install nginx
    ```

2.  **Configure Nginx:**
    *   Create a new Nginx configuration file for your application:
        ```bash
        sudo nano /etc/nginx/sites-available/whatsapp-bot
        ```
    *   Add the following configuration to the file, replacing `<your_domain>` with your domain name or your droplet's IP address:
        ```nginx
        server {
            listen 80;
            server_name <your_domain>;

            location / {
                proxy_pass http://localhost:8080;
                proxy_set_header Host $host;
                proxy_set_header X-Real-IP $remote_addr;
                proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
                proxy_set_header X-Forwarded-Proto $scheme;
            }
        }
        ```
    *   Enable the new configuration by creating a symbolic link:
        ```bash
        sudo ln -s /etc/nginx/sites-available/whatsapp-bot /etc/nginx/sites-enabled/
        ```
    *   Test your Nginx configuration:
        ```bash
        sudo nginx -t
        ```
    *   If the test is successful, restart Nginx:
        ```bash
        sudo systemctl restart nginx
        ```

3.  **Set up SSL with Let's Encrypt (Recommended):**
    *   Install Certbot, the Let's Encrypt client:
        ```bash
        sudo apt install certbot python3-certbot-nginx
        ```
    *   Obtain an SSL certificate for your domain:
        ```bash
        sudo certbot --nginx -d <your_domain>
        ```
    *   Certbot will automatically configure Nginx to use the SSL certificate and will set up automatic renewal.

## Step 4: Install Go and Your Application

1.  **Install Go:**
    *   Download the latest version of Go from the [official website](https://golang.org/dl/):
        ```bash
        wget https://golang.org/dl/go1.21.5.linux-amd64.tar.gz
        ```
    *   Extract the archive:
        ```bash
        sudo tar -C /usr/local -xzf go1.21.5.linux-amd64.tar.gz
        ```
    *   Add the Go binary to your `PATH` environment variable. Open your `~/.profile` file:
        ```bash
        nano ~/.profile
        ```
    *   Add the following line to the end of the file:
        ```bash
        export PATH=$PATH:/usr/local/go/bin
        ```
    *   Apply the changes:
        ```bash
        source ~/.profile
        ```

2.  **Clone your application:**
    *   Clone your application from your Git repository:
        ```bash
        git clone <your_git_repository_url>
        ```

## Step 5: Configure and Run Your Application

1.  **Set up your environment variables:**
    *   Navigate to your application's directory:
        ```bash
        cd <your_application_directory>
        ```
    *   Create a `.env` file:
        ```bash
        nano .env
        ```
    *   Add your environment variables to the file:
        ```
        WHATSAPP_PHONE_NUMBER_ID=<your_phone_number_id>
        WHATSAPP_ACCESS_TOKEN=<your_access_token>
        WHATSAPP_WEBHOOK_VERIFY_TOKEN=<your_verify_token>
        # Add any other environment variables your application needs
        ```

2.  **Run your application as a background service:**
    *   Create a `systemd` service file for your application:
        ```bash
        sudo nano /etc/systemd/system/whatsapp-bot.service
        ```
    *   Add the following configuration to the file, replacing `<your_application_directory>` with the path to your application:
        ```ini
        [Unit]
        Description=WhatsApp Bot
        After=network.target

        [Service]
        User=root
        WorkingDirectory=<your_application_directory>
        ExecStart=/usr/local/go/bin/go run main.go
        Restart=always

        [Install]
        WantedBy=multi-user.target
        ```
    *   Reload the `systemd` daemon:
        ```bash
        sudo systemctl daemon-reload
        ```
    *   Start your application:
        ```bash
        sudo systemctl start whatsapp-bot
        ```
    *   Enable your application to start on boot:
        ```bash
        sudo systemctl enable whatsapp-bot
        ```

## Step 6: Configure Your WhatsApp Webhook

1.  **Configure the webhook in your Facebook Developer App:**
    *   In your Facebook Developer App, go to the "WhatsApp" -> "Configuration" section.
    *   Click on "Edit" in the "Webhook" section.
    *   Set the "Callback URL" to your domain name or your droplet's IP address, followed by the webhook path (e.g., `https://<your_domain>/api/v1/whatsapp/webhook`).
    *   Set the "Verify Token" to the value you've set for the `WHATSAPP_WEBHOOK_VERIFY_TOKEN` environment variable.
    *   Click "Verify and Save".

2.  **Subscribe to webhook events:**
    *   After verifying your webhook, you'll need to subscribe to the "messages" event.

## Step 7: Test Your Bot

1.  **Send a message to your test number:**
    *   Using your personal WhatsApp account, send a message to the test phone number you received from Facebook.
    *   The message should be in the format `start <campaign_id>`, where `<campaign_id>` is a valid campaign ID from your database.

If everything is set up correctly, your application should receive the webhook event, and the WhatsApp bot should respond with the first message in the conversation flow.
