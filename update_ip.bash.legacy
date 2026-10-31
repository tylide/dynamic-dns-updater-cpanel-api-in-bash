#!/usr/bin/env bash

if [ -f .env ]; then
    export $(grep -v '^#' .env | xargs)
else
    echo ".env file not found!"
    exit 1
fi

function get_ip() {
    curl -s $GET_IP_URL 
}

get_domain_ip() {
    local url=$1
    local ip=$(dig +short "$url")

    if [ -z "$ip" ]; then
        echo "Unable to resolve IP for $url"
        return 1
    else
        echo "$ip"
        return 0
    fi
}

function is_online() {
    local url=$1
    local status=$(curl -Is "$url" | grep HTTP/ | awk '{print $2}')
    
    if [[ $status -ge 200 && $status -lt 300 ]]; then
        return 0
    else
        return 1
    fi
}

function is_installed() {
    local program=$1
    if command -v "$program" &> /dev/null; then
        return 0
    else
        return 1
    fi
}


function is_defined() {
    local var_name=$1
    if [ -z "${!var_name}" ]; then
        return 1
    else
        return 0
    fi
}

function ntfy() {
    #priority max/urgent hight default low min
    #tags https://docs.ntfy.sh/emojis/
    local title=$1
    local priority=$2
    local tags=$3
    local message=$4
    local url=$NTFY_SERVER
    local topic=$NTFY_TOPIC
    local token=$NTFY_TOKEN

    if [[ ! "$#" -eq 4 ]]; then
        echo "usage: ntfy title priority tags message" >&2
        return 1
    fi

    local VARS=(NTFY_SERVER NTFY_TOPIC NTFY_TOKEN)

    for var in ${VARS[@]}; do
      if ! is_defined $var; then
        echo "To send ntfy define $var in .env file"
        return 1
      fi
    done
    
    curl -s \
        -H "Authorization: Bearer $token" \
        -H "Title: $title" \
        -H "Priority: $priority" \
        -H "Tags: $tags" \
        -d "$message" \
        $url/$topic > /dev/null 2>&1
}

function get_zone() {
    zone=$(curl -s -H "Authorization: cpanel $USERNAME:$APIKEY" \
        "$CPANEL_URL:$CPANEL_PORT/execute/DNS/parse_zone?zone=$DOMAIN")

    echo $(parse_result $zone)
}

function parse_result() {
    local json_string="$1"
    local data

    data=$(echo "$json_string" | jq -r 'select(. != null) | .data')

    if [ "$data" == "null" ]; then
        return 1
    fi

    echo "$data" | jq -c '.'
}

function find_record() {
    local data="$1"
    local target_dname="$2"

    local target_dname_b64=$(echo -n "$target_dname" | base64)

    if ! echo "$data" | jq empty > /dev/null 2>&1; then
        echo "JSON inválido."
        return 1
    fi

    local result=$(echo "$data" | jq -r --arg target "$target_dname_b64" '
        map(select(.dname_b64 == $target)) | .[] | {
            line_index: .line_index,
            record_type: .record_type,
            ip: (.data_b64[0] | @base64d)
        } // empty
    ')

    if [ -n "$result" ]; then
        local line_index=$(echo "$result" | jq -r '.line_index')
        local ip=$(echo "$result" | jq -r '.ip')
        local record_type=$(echo "$result" | jq -r '.record_type')

        echo "{\"line_index\": $line_index, \"dname\":\"$target_dname\", \"ttl\":$TTL, \"record_type\":\"$record_type\", \"data\":[\"$ip\"]}"
        return 0
    else
        echo "Nenhum registro encontrado com dname_b64 igual a '$target_dname_b64'."
        return 1
    fi
}

function last_serial() {
    local data="$1"

    # Verifica se a string é um JSON válido
    if ! echo "$data" | jq empty > /dev/null 2>&1; then
        echo "JSON inválido."
        return 1
    fi

    # Filtra o registro do tipo "SOA" e obtém o campo data_b64[2]
    local result=$(echo "$data" | jq -r 'map(select(.record_type == "SOA")) | .[].data_b64[2] // empty')

    # Verifica se o resultado não está vazio
    if [ -n "$result" ]; then
        echo "$result" | base64 -d
        return 0
    else
        echo "Nenhum registro do tipo 'SOA' encontrado."
        return 1
    fi
}

function change_zone() {
    local json resposta status error serial correct_serial operation operations
    operations_json='{"operations": ["add", "edit"]}'
    operation=$1

    if ! echo "$operations_json" | jq -e --arg op "$operation" '.operations | index($op)' > /dev/null; then
        echo "Operation '$operation' is not valid."
        return 1
    fi

    json=$2
    serial=$3

    resposta=$(curl -s -H \
        "Authorization: cpanel $USERNAME:$APIKEY" \
        -d "zone=$DOMAIN" -d "serial=$serial" -d "$operation=$json" \
        "$CPANEL_URL:$CPANEL_PORT/execute/DNS/mass_edit_zone")

    status=$(echo $resposta | jq '.status')

    if [[ $status -eq 0 ]]; then
        error=$(echo $resposta | jq '.errors')
        echo $error
        return
    fi
}

function verify() {
    local PROGRAMS=(curl jq)
    local SERVERS=($GET_IP_URL $CPANEL_URL)
    local VARS=(CPANEL_URL CPANEL_PORT USERNAME APIKEY DOMAIN TTL GET_IP_URL SUBDOMAIN)

    for program in ${PROGRAMS[@]}; do
        if ! is_installed jq; then
            echo "Jq não instalado!" >&2
            return 1
        fi
    done

    for server in ${SERVERS[@]}; do
        if ! is_online $server; then
            echo "The server $server is offline" >&2
            ntfy "Server Offline" "max" "no_entry" "The server $server is offline"
            return 1
        fi
    done

    for var in ${VARS[@]}; do
      if ! is_defined $var; then
        echo "Var $var is not set on .env file" >&2
        return 1
      fi
    done

    return 0
}

LOCK_FILE="$(dirname "$(realpath "$0")")/.lock_update"

if [[ -f $LOCK_FILE ]]; then
    echo "Update locked, to unlock remote '$LOCK_FILE'" >&2
    exit 1
fi

if ! verify; then
    exit 1
fi

operation="edit"
public_ip=$(get_ip)
zone=$(get_zone)
last_serial=$(last_serial "$zone")
record_found=$(find_record "$zone" $SUBDOMAIN)

if ! echo "$record_found" | jq empty > /dev/null 2>&1; then
    record="{\"dname\":\"$SUBDOMAIN\",\"ttl\":$TTL,\"record_type\":\"A\",\"data\":[\"$public_ip\"]}"
    operation="add"
fi

if [[ $operation == 'edit' ]]; then
    domain_ip=$(echo $record_found | jq -c '.data[0]' | sed 's/\"//g')
    if [[ "$public_ip" != "$domain_ip" ]]; then
        record=$(echo "$record_found" | jq "(.data[0] |= if . != \"$public_ip\" then \"$public_ip\" else . end)")
        update=1
    fi
fi

if [[ $operation == add ]] || [[ ! -z $update ]]; then
    touch "$LOCK_FILE"
    change_zone "$operation" "$record" "$last_serial"

    if [[ $operation == add ]]; then
        title="New subdomain added"
        tags="heavy_plus_sign"
    else
        title="Subdomain updated"
        tags="up"
    fi

    ntfy "$title" "hight" "$tags" "Subdomain: $SUBDOMAIN on IP: $public_ip"
    rm "$LOCK_FILE"
fi

