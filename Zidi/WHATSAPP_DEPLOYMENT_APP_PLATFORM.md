# Deploying Your WhatsApp Bot on DigitalOcean App Platform

This guide will walk you through the entire process of deploying your Go-based WhatsApp bot, from creating a Facebook App to deploying on DigitalOcean's App Platform.

## Part 1: Setting Up Your Facebook App and WhatsApp Business API

### Step 1: Create a Facebook Business Account

1.  Go to [business.facebook.com/overview](https://business.facebook.com/overview).
2.  Click **Create account**.
3.  Enter a name for your business, your name, and your work email address, and then click **Next**.
4.  Enter your business details and click **Submit**.

### Step 2: Create a Facebook Developer App

1.  Go to the [Facebook for Developers](https://developers.facebook.com/) website and click on **My Apps**.
2.  Click **Create App**.
3.  Select **Business** as the app type and click **Next**.
4.  Give your app a name, and select your Business Account from the dropdown menu.
5.  Click **Create App**. You might be asked to re-enter your Facebook password.

### Step 3: Set Up the WhatsApp Business API

1.  From your app's dashboard, scroll down and find the **WhatsApp** product. Click **Set up**.
2.  You will be asked to select your Business Account. Select the one you created in Step 1 and click **Continue**.
3.  You will now be taken to the WhatsApp Business Platform. Here, you will see a temporary test phone number that you can use to send and receive messages. You will also see a **Phone number ID** and a **WhatsApp Business Account ID**. You will need these later.
4.  You will also see a temporary **Access Token**. This token is valid for 23 hours. You will need this token to send messages.

### Step 4: Add a "To" Phone Number

1.  In the "To" field, select the country code and enter your phone number.
2.  Click **Send Code**. You will receive a verification code on your phone.
3.  Enter the verification code to verify your phone number.

## Part 2: Deploying Your Bot on DigitalOcean App Platform

### Step 1: Prepare Your Application for Deployment

1.  **Ensure your application listens on the correct port:** DigitalOcean App Platform will set the `PORT` environment variable to tell your application which port to listen on. Your application should use this environment variable if it's available, and fall back to a default port (e.g., 8080) if it's not.

    In your `main.go` file, you can modify the `app.Listen` call to do this:

    ```go
    port := os.Getenv("PORT")
    if port == "" {
        port = "8080"
    }
    log.Fatal(app.Listen(":" + port))
    ```

2.  **Make sure all dependencies are in your `go.mod` file:** Run `go mod tidy` to ensure your `go.mod` and `go.sum` files are up to date.

### Step 2: Create a New App on DigitalOcean App Platform

1.  Go to your DigitalOcean dashboard and click on **Apps** in the left-hand menu.
2.  Click **Create App**.
3.  Choose your Git repository provider (e.g., GitHub) and select your application's repository.
4.  Choose the branch you want to deploy (e.g., `main`) and click **Next**.
5.  App Platform will automatically detect that you have a Go application. It will suggest a build command (`go build`) and a run command (`./<your_app_name>`). These should be correct.
6.  Click **Next**.

### Step 3: Configure Your App's Environment Variables

1.  In the "Environment Variables" section, click **Edit**.
2.  Add the following environment variables:
    *   `WHATSAPP_PHONE_NUMBER_ID`: Your WhatsApp Business phone number ID.
    *   `WHATSAPP_ACCESS_TOKEN`: Your WhatsApp Business access token.
    *   `WHATSAPP_WEBHOOK_VERIFY_TOKEN`: Your webhook verification token.
    *   Add any other environment variables your application needs.
3.  Click **Save**.

### Step 4: Deploy Your App

1.  Choose a plan for your app. The "Basic" plan should be sufficient for testing.
2.  Click **Create Resources**.
3.  DigitalOcean will now build and deploy your application. You can monitor the progress in the "Deployments" tab.

### Step 5: Get Your App's Public URL

Once your app is deployed, you'll see its public URL at the top of the app's dashboard. It will look something like this: `https://<your-app-name>-<random-string>.ondigitalocean.app`.

## Part 3: Configuring Your WhatsApp Webhook and Testing Your Bot

### Step 1: Configure Your WhatsApp Webhook

1.  In your Facebook Developer App, go to the "WhatsApp" -> "Configuration" section.
2.  Click on "Edit" in the "Webhook" section.
3.  Set the "Callback URL" to your DigitalOcean App Platform URL, followed by the webhook path (e.g., `https://<your-app-name>-<random-string>.ondigitalocean.app/api/v1/whatsapp/webhook`).
4.  Set the "Verify Token" to the value you've set for the `WHATSAPP_WEBHOOK_VERIFY_TOKEN` environment variable.
5.  Click **Verify and Save**.

### Step 2: Test Your Bot

1.  Send a message to your test number from your personal WhatsApp account.
2.  The message should be in the format `start <campaign_id>`, where `<campaign_id>` is a valid campaign ID from your database.

If everything is set up correctly, your application should receive the webhook event, and the WhatsApp bot should respond with the first message in the conversation flow.
