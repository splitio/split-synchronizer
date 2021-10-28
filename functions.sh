#!/bin/env sh

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


