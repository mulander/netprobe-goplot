package constants

const (
  EXIT_SUCCESS = 0;
  EXIT_NO_CONFIG; // config file not found or couldn't be read
  EXIT_CONFIG_PARSE; // failed to parse the config file
  EXIT_CANT_LISTEN;
)