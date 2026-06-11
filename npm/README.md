# @game1991/cc-connect

Fork of cc-connect with role-based tool permissions and daemon fixes.

## Install

### Option 1: GitHub Packages (recommended for teams)

```bash
# Configure npm to use GitHub Packages for this scope
echo "@game1991:registry=https://npm.pkg.github.com" >> ~/.npmrc

# Install (requires GitHub PAT with read:packages scope)
npm install -g @game1991/cc-connect
```

### Option 2: GitHub direct install

```bash
npm install -g github:game1991/cc-connect#feat/role-based-tool-permissions
```

### Option 3: Manual binary replace

See [docs/fork-verify-flow.html](../docs/fork-verify-flow.html) for the full tutorial.

## Usage

```bash
cc-connect --version
cc-connect daemon install
cc-connect daemon status
```

## Documentation

- Fork: https://github.com/game1991/cc-connect
- Upstream: https://github.com/chenhg5/cc-connect
