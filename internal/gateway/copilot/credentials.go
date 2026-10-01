package copilot

import (
	"context"
	"errors"
	"os/exec"
	"strings"
)

const keychainService = "copilot-cli"

func (source *Source) resolveToken(ctx context.Context, config configFile, user configUser) (string, error) {
	if token := configToken(config, user.Host, user.Login); token != "" {
		return token, nil
	}
	account := accountKey(user.Host, user.Login)
	for _, resolver := range []func(context.Context) (string, error){
		func(ctx context.Context) (string, error) { return source.readKeytarToken(ctx, account) },
		func(ctx context.Context) (string, error) { return source.readLibsecretToken(ctx, account) },
		func(ctx context.Context) (string, error) { return source.readSDKToken(ctx, user.Host, user.Login) },
	} {
		token, err := resolver(ctx)
		if err == nil && strings.TrimSpace(token) != "" {
			return strings.TrimSpace(token), nil
		}
	}
	return "", errors.New("failed to resolve stored Copilot token from config or keychain")
}

func (source *Source) readKeytarToken(ctx context.Context, account string) (string, error) {
	path, err := source.discoverKeytarPath()
	if err != nil {
		return "", err
	}
	script := `const keytar=require(process.argv[1]);keytar.getPassword(process.argv[2],process.argv[3]).then(token=>process.stdout.write(token||''),()=>process.exit(1));`
	result, err := source.run(ctx, "node", []string{"-e", script, path, keychainService, account})
	if err != nil || result.exitCode != 0 {
		return "", errors.New("Copilot keytar lookup failed")
	}
	return strings.TrimSpace(string(result.stdout)), nil
}

func (source *Source) readLibsecretToken(ctx context.Context, account string) (string, error) {
	for _, attribute := range []string{"username", "account"} {
		result, err := source.run(ctx, "secret-tool", []string{"lookup", "service", keychainService, attribute, account})
		if err == nil && result.exitCode == 0 {
			if token := strings.TrimSpace(string(result.stdout)); token != "" {
				return token, nil
			}
		}
	}
	return "", errors.New("Copilot libsecret lookup failed")
}

func (source *Source) readSDKToken(ctx context.Context, host, login string) (string, error) {
	sdkPath, err := source.discoverSDKPath()
	if err != nil {
		return "", err
	}
	binary := source.copilotBinary()
	script := `const {CopilotClient,RuntimeConnection}=await import(process.argv[1]);const client=new CopilotClient({connection:RuntimeConnection.forStdio({path:process.argv[2]}),logLevel:'none'});try{await client.start();const users=await client.rpc.account.getAllUsers();const normalize=v=>String(v||'').replace(/\/+$/,'').toLowerCase();const host=normalize(process.argv[3]);const login=process.argv[4];const user=users.find(({authInfo})=>normalize(authInfo?.host)===host&&authInfo?.login===login);const token=typeof user?.token==='string'?user.token.trim():'';if(token)process.stdout.write(token);}finally{try{await client.stop();}catch{}}`
	result, err := source.run(ctx, "node", []string{"--input-type=module", "-e", script, sdkPath, binary, host, login})
	if err != nil || result.exitCode != 0 {
		return "", errors.New("Copilot SDK lookup failed")
	}
	return strings.TrimSpace(string(result.stdout)), nil
}

func (source *Source) copilotBinary() string {
	if override := strings.TrimSpace(source.getenv("PRODEX_COPILOT_BIN")); override != "" {
		return override
	}
	if path, err := exec.LookPath("copilot"); err == nil {
		return path
	}
	return "copilot"
}
