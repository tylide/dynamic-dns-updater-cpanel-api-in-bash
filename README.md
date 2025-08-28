A **Dynamic DNS Updater for cPanel** is a Bash script that automatically updates your DNS records on cPanel whenever your public IP address changes. This tool is particularly useful for users with dynamic IP addresses who want to ensure their domain always points to the correct IP. The script retrieves the current public IP, checks it against the existing DNS record, and updates the record if there is a discrepancy.

### Installation and Usage Guide

#### Prerequisites

- A cPanel account with API access.
- `curl` and `jq` installed on your system.
- A `.env` file containing your cPanel credentials and configuration.

#### Installation Steps

1. **Clone the Repository**:
   Open your terminal and run the following command to clone the repository:

   ```bash
   git clone https://github.com/tylide/dynamic-dns-updater-cpanel-api-in-bash.git
   cd dynamic-dns-updater-cpanel-api-in-bash
   ```

2. **Create a `.env` File**:
   Create a `.env` file in the root directory of the cloned repository and add your cPanel configuration:

   ```plaintext
   CPANEL_URL=https://your-cpanel-url
   CPANEL_PORT=2083
   USERNAME=your-username
   APIKEY=your-api-key
   DOMAIN=your-domain
   TTL=300
   GET_IP_URL=checkip.amazonaws.com
   SUBDOMAIN=subdomain
   NTFY_SERVER=https://ntfy.sh
   NTFY_TOPIC=dnsupdate
   NTFY_TOKEN=tk_my_ntfy_token
   ```

3. **Make the Script Executable**:
   Change the permissions of the script to make it executable:

   ```bash
   chmod +x update_ip.sh
   ```

#### Usage

To run the script manually, execute the following command in your terminal:

```bash
./update_ip.sh
```

#### Automating with Cron

To ensure that your DNS records are updated automatically, you can set up a cron job:

1. **Open the Crontab**:
   Run the following command to edit your crontab:

   ```bash
   crontab -e
   ```

2. **Add a Cron Job**:
   Add the following line to run the script every 5 minutes (adjust the timing as needed):

   ```plaintext
   */5 * * * * /path/to/your/dynamic-dns-updater-cpanel-api-in-bash/update_ip.sh
   ```

   Replace `/path/to/your/dynamic-dns-updater-cpanel-api-in-bash/` with the actual path to your script.

3. **Save and Exit**:
   Save the changes and exit the editor. Your cron job is now set up!

### Conclusion

The **Dynamic DNS Updater for cPanel** is a simple yet effective solution for keeping your DNS records in sync with your dynamic IP address. With easy installation and automation through cron, you can ensure your domain always points to the correct IP.

Feel free to contribute to the project or report any issues you encounter!
