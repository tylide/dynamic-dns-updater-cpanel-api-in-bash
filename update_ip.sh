#!/usr/bin/env bash

if [ -f .env ]; then
    export $(grep -v '^#' .env | xargs)
else
    echo ".env file not found!"
    exit 1
fi

function get_ip() {
    curl $GET_IP_URL 2> /dev/null
}

function domain_current_ip() {
    local DOMAIN=$1
    local resposta=$(curl -H "Authorization: cpanel $USERNAME:$APIKEY" "$CPANEL_URL:$CPANEL_PORT/execute/DNS/lookup" -d "domain=$DOMAIN" 2> /dev/null)
    
    echo $resposta | grep -E '[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+' -o
}

function last_serial() {
    curl -H "Authorization: cpanel $USERNAME:$APIKEY" \
        "$CPANEL_URL:$CPANEL_PORT/execute/DNS/parse_zone?zone=$DOMAIN" 2> /dev/null \
        | jq '.data[3].data_b64[2]' \
        | sed 's/\"//g' \
        | base64 -d
}

function update_domain_ip() {
    local dname=$1
    local line_index=$2
    local ip=$3
    local ttl=$4

    local json resposta status error serial correct_serial

    serial=$(last_serial)
    
    json="{\"line_index\": $line_index, \"dname\":\"$dname\", \"ttl\":$ttl, \"record_type\":\"A\", \"data\":[\"$ip\"]}"

    resposta=$(curl -H "Authorization: cpanel $USERNAME:$APIKEY" "$CPANEL_URL:$CPANEL_PORT/execute/DNS/mass_edit_zone" -d "zone=$DOMAIN" -d "serial=$serial" -d "edit=$json" 2> /dev/null)

    status=$(echo $resposta | jq '.status')

    if [[ $status -eq 0 ]]; then
        error=$(echo $resposta | jq '.errors')
        echo $error
        return
    fi
    
    echo $resposta | grep -E 'serial\":\"[0-9]+\"' -o | grep -E '[0-9]+' -o > last_serial
}


public_ip=$(get_ip)

domain_ip=$(domain_current_ip $DNAME.$DOMAIN)

if [[ "$public_ip" != "$domain_ip" ]]; then
    update_domain_ip $DNAME $LINDEX $public_ip "300"
fi

