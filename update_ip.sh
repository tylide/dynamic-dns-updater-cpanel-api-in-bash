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
    operations='{"operations": ["add", "edit"]}'
    operation=$1

    if ! echo "$operations_json" | jq -e --arg op "$operation" '.operations | index($op)' > /dev/null; then
        echo "Operation '$operation' is not valid."
        return 1
    fi

    json=$2
    serial=$3

    resposta=$(curl -s -H "Authorization: cpanel $USERNAME:$APIKEY" "$CPANEL_URL:$CPANEL_PORT/execute/DNS/mass_edit_zone" -d "zone=$DOMAIN" -d "serial=$serial" -d "$operation=$json")

    status=$(echo $resposta | jq '.status')

    if [[ $status -eq 0 ]]; then
        error=$(echo $resposta | jq '.errors')
        echo $error
        return
    fi
}


public_ip=$(get_ip)
zone=$(get_zone)
last_serial=$(last_serial "$zone")
operation="edit"

record_found=$(find_record "$zone" $SUBDOMAIN)

if ! echo "$record_found" | jq empty > /dev/null 2>&1; then
    record="{\"dname\":\"$SUBDOMAIN\",\"ttl\":300,\"record_type\":\"A\",\"data\":[\"$public_ip\"]}"
    operation="add"
fi

if [[ $operation == 'edit' ]]; then
    domain_ip=$(echo $record_found | jq -c '.data[0]' | sed 's/\"//g')
    if [[ "$public_ip" != "$domain_ip" ]]; then
        record=$(echo "$record_found" | jq "(.data[0] |= if . != \"$public_ip\" then \"$public_ip\" else . end)")
    fi
    update=1
fi

if [[ $operation == add ]] || [[ ! -z $update ]]; then
    change_zone "$operation" "$record" "$last_serial"
fi

