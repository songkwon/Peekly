//go:build darwin && cgo

#import <Foundation/Foundation.h>
#import <Security/Security.h>
#import <SystemConfiguration/SystemConfiguration.h>
#include <stdlib.h>
#include <string.h>

static AuthorizationRef authorization;

static NSArray<NSString *> *managedKeys(void) {
    return @[@"HTTPEnable", @"HTTPProxy", @"HTTPPort", @"HTTPSEnable",
        @"HTTPSProxy", @"HTTPSPort", @"ProxyAutoConfigEnable", @"ProxyAutoDiscoveryEnable"];
}

static NSDictionary *managedSettings(NSDictionary *proxies) {
    NSMutableDictionary *managed = [NSMutableDictionary dictionary];
    for (NSString *key in managedKeys()) {
        managed[key] = proxies[key] ?: [NSNull null];
    }
    return managed;
}

static char *copyJSON(NSDictionary *values) {
    NSData *data = [NSJSONSerialization dataWithJSONObject:values options:NSJSONWritingSortedKeys error:nil];
    if (!data) {
        return NULL;
    }
    return strdup([[[NSString alloc] initWithData:data encoding:NSUTF8StringEncoding] autorelease].UTF8String);
}

static char *errorString(int status) {
    const char *description = SCErrorString(status);
    return strdup(description ?: "System Configuration error");
}

static SCNetworkProtocolRef copyProxyProtocol(SCPreferencesRef prefs, NSString *serviceID) {
    SCNetworkServiceRef service = SCNetworkServiceCopy(prefs, (__bridge CFStringRef)serviceID);
    if (!service) {
        return NULL;
    }
    SCNetworkProtocolRef proxy = SCNetworkServiceCopyProtocol(service, kSCNetworkProtocolTypeProxies);
    CFRelease(service);
    return proxy;
}

static NSString *activeService(void) {
    for (NSString *key in @[@"State:/Network/Global/IPv4", @"State:/Network/Global/IPv6"]) {
        CFPropertyListRef value = SCDynamicStoreCopyValue(NULL, (__bridge CFStringRef)key);
        if (value) {
            NSString *service = [((__bridge NSDictionary *)value)[@"PrimaryService"] retain];
            CFRelease(value);
            if (service) {
                return [service autorelease];
            }
        }
    }
    return nil;
}

char *peekly_read_proxy(const char *rawService, char **message) {
    @autoreleasepool {
        NSString *service = rawService[0] ? [NSString stringWithUTF8String:rawService] : activeService();
        if (!service) {
            *message = strdup("No active Wi-Fi or Ethernet network service was found");
            return NULL;
        }
        SCPreferencesRef prefs = SCPreferencesCreate(NULL, CFSTR("Peekly"), NULL);
        if (!prefs) {
            *message = errorString(SCError());
            return NULL;
        }
        SCNetworkProtocolRef proxy = copyProxyProtocol(prefs, service);
        CFRelease(prefs);
        if (!proxy) {
            *message = strdup("network-service-missing");
            return NULL;
        }
        NSDictionary *config = (__bridge NSDictionary *)SCNetworkProtocolGetConfiguration(proxy);
        char *result = copyJSON(@{@"service": service, @"settings": managedSettings(config)});
        CFRelease(proxy);
        if (!result) {
            *message = strdup("Unable to read current proxy settings");
        }
        return result;
    }
}

static BOOL obtainRights(BOOL interactive) {
    if (!authorization && AuthorizationCreate(NULL, NULL, kAuthorizationFlagDefaults, &authorization) != errAuthorizationSuccess) {
        return NO;
    }
    AuthorizationItem item = {"system.services.systemconfiguration.network", 0, NULL, 0};
    AuthorizationRights rights = {1, &item};
    AuthorizationFlags flags = kAuthorizationFlagExtendRights;
    if (interactive) {
        flags |= kAuthorizationFlagInteractionAllowed;
    }
    return AuthorizationCopyRights(authorization, &rights, NULL, flags, NULL) == errAuthorizationSuccess;
}

char *peekly_replace_proxy(const char *rawService, const char *rawExpected, const char *rawNew, int interactive) {
    @autoreleasepool {
        NSString *service = [NSString stringWithUTF8String:rawService];
        NSDictionary *expected = [NSJSONSerialization JSONObjectWithData:[NSData dataWithBytes:rawExpected length:strlen(rawExpected)] options:0 error:nil];
        NSDictionary *desired = [NSJSONSerialization JSONObjectWithData:[NSData dataWithBytes:rawNew length:strlen(rawNew)] options:0 error:nil];
        if (!service || !expected || !desired) {
            return strdup("Invalid proxy settings");
        }
        if (!obtainRights(interactive)) {
            return strdup("macOS has not authorized changes to network settings");
        }
        SCPreferencesRef prefs = SCPreferencesCreateWithAuthorization(NULL, CFSTR("Peekly"), NULL, authorization);
        if (!prefs) {
            return errorString(SCError());
        }
        if (!SCPreferencesLock(prefs, false)) {
            char *message = errorString(SCError());
            CFRelease(prefs);
            return message;
        }
        SCPreferencesSynchronize(prefs);
        SCNetworkProtocolRef proxy = copyProxyProtocol(prefs, service);
        char *message = NULL;
        if (!proxy) {
            message = strdup("network-service-missing");
        } else {
            NSDictionary *current = (__bridge NSDictionary *)SCNetworkProtocolGetConfiguration(proxy);
            if (![managedSettings(current) isEqual:expected]) {
                message = strdup("proxy-settings-changed");
            } else {
                NSMutableDictionary *next = current ? [[current mutableCopy] autorelease] : [NSMutableDictionary dictionary];
                for (NSString *key in managedKeys()) {
                    id value = desired[key];
                    if (!value || value == [NSNull null]) {
                        [next removeObjectForKey:key];
                    } else {
                        next[key] = value;
                    }
                }
                if (!SCNetworkProtocolSetConfiguration(proxy, (__bridge CFDictionaryRef)next)
                    || !SCPreferencesCommitChanges(prefs)
                    || !SCPreferencesApplyChanges(prefs)) {
                    message = errorString(SCError());
                }
            }
            CFRelease(proxy);
        }
        SCPreferencesUnlock(prefs);
        CFRelease(prefs);
        return message;
    }
}
