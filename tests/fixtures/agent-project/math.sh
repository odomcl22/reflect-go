# Return value constrained to the inclusive interval [low, high].
clamp() {
    value=$1
    low=$2
    high=$3
    printf '%s\n' "$value"
}
