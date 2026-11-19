{
  config,
  pkgs,
  lib,
  ...
}:

{
  name = "Pageroom";

  # https://devenv.sh/packages/
  packages = [
    pkgs.go-task
    pkgs.golangci-lint
    pkgs.hugo
  ]
  ++ lib.optionals pkgs.stdenv.isLinux [
    pkgs.gtk4
    pkgs.pkg-config
    pkgs.webkitgtk_6_0
  ];

  # https://devenv.sh/languages/
  languages.go.enable = true;
  languages.javascript = {
    enable = true;
    package = pkgs.nodejs_22;
    npm.enable = true;
  };

  # Chromium for `task screenshots`. The playwright-core pin in frontend/package.json must use the same Chromium revision.
  env.PLAYWRIGHT_BROWSERS_PATH = pkgs.playwright-driver.browsers;
  env.PLAYWRIGHT_SKIP_VALIDATE_HOST_REQUIREMENTS = "true";

  # A nixpkgs update can move Chromium away from the pin, and Playwright only notices halfway through `task screenshots`.
  enterShell = ''
    wanted=$(node -e 'const browsers = JSON.parse(require("fs").readFileSync("frontend/node_modules/playwright-core/browsers.json", "utf8")).browsers; console.log(browsers.find((browser) => browser.name === "chromium-headless-shell").revision)' 2>/dev/null)
    if [ -n "$wanted" ] && [ ! -d "$PLAYWRIGHT_BROWSERS_PATH/chromium_headless_shell-$wanted" ]; then
      shipped=$(ls "$PLAYWRIGHT_BROWSERS_PATH" | grep '^chromium_headless_shell-' | cut -d- -f2)
      echo "Warning. playwright-core in frontend/package.json wants Chromium $wanted, but devenv has $shipped. Pin the playwright-core version that uses $shipped."
    fi
  '';

  # nixpkgs lags behind the Wails version in go.mod, and the CLI must match it.
  scripts.wails3.exec = ''go tool wails3 "$@"'';

  # Run them all with `task lint`.
  # https://devenv.sh/git-hooks/
  git-hooks.hooks = {
    gofmt.enable = true;
    govet.enable = true;
    # Japanese terms the UI avoids.
    japanese-terms = {
      enable = true;
      name = "japanese-terms";
      entry = ''${pkgs.bash}/bin/bash -c '! grep -nF -e 既定 -e 非公開 -e ウェブサイト -e チームメイトさん "$@"' --'';
      files = "^(frontend/src/i18n/ja\\.tsx|internal/site/contactform\\.go)$";
    };
    go-mod-tidy = {
      enable = true;
      name = "go-mod-tidy";
      entry = "${config.languages.go.package}/bin/go mod tidy -diff";
      files = "^(go\\.mod|go\\.sum|.*\\.go)$";
      pass_filenames = false;
    };
    # Only errcheck and gocyclo, set in .golangci.yml.
    golangci-lint = {
      enable = true;
      name = "golangci-lint";
      entry = "${pkgs.golangci-lint}/bin/golangci-lint run ./...";
      files = "^(\\.golangci\\.yml|go\\.mod|.*\\.go)$";
      pass_filenames = false;
    };
    nixfmt.enable = true;
  };

  # See full reference at https://devenv.sh/reference/options/
}
