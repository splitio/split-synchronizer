#!/bin/env bash

function parse_flags_from_conf_file() {
    fn=$1

    regex=".*s-cli:\"([^ ]+)\" .*"
    while IFS="" read -r line || [ -n "$line" ]; do
        if [[ $line =~ $regex ]]; then
            name="${BASH_REMATCH[1]}"
            echo $name
        fi
    done < $fn
}

function flag_to_env_var() {
    prefix=$1
    flag=$2

    if [ "$prefix" == "" ] || [ "$flag" == "" ]; then
        return 1
    fi

    echo "${prefix}_${flag}" | tr "[a-z]" "[A-Z]" | tr "-" "_"
    return 0
}

# ack 's-cli:([^ ]*) ' --output '$1' sections.go
function parse_env() {
    prefix=$1
    flags=$2

    if [ "$prefix" == "" ]; then
        return 1
    fi

    args=""
    for idx in ${!flags[@]}; do
        flag=${flags[idx]}
        env=$(flag_to_env_var "$prefix" "$flag")
        if [ $? -ne 0 ]; then
            continue
        fi

        if [ ! -z ${!env+x} ]; then
          args="${args} -${flag}=${!env}"
        fi
    done

    echo $args
}


