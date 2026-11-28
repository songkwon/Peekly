//go:build darwin && cgo

#import <AppKit/AppKit.h>
#import <libproc.h>
#import <string.h>

char *peekly_process_bundle(int pid) {
    @autoreleasepool {
        char image[PROC_PIDPATHINFO_MAXSIZE];
        if (proc_pidpath(pid, image, sizeof(image)) <= 0) {
            return NULL;
        }
        NSString *path = [[NSFileManager defaultManager] stringWithFileSystemRepresentation:image length:strlen(image)];
        NSString *bundle = @"";
        for (NSString *component in [path pathComponents]) {
            bundle = [bundle stringByAppendingPathComponent:component];
            if ([[[component pathExtension] lowercaseString] isEqualToString:@"app"]) {
                return strdup([bundle fileSystemRepresentation]);
            }
        }
        return NULL;
    }
}

void peekly_application_details(const char *path, char **name, char **png) {
    @autoreleasepool {
        NSString *location = [[NSFileManager defaultManager] stringWithFileSystemRepresentation:path length:strlen(path)];
        NSBundle *bundle = [NSBundle bundleWithPath:location];
        if (!bundle) {
            return;
        }
        NSString *title = [bundle objectForInfoDictionaryKey:@"CFBundleDisplayName"];
        if (![title isKindOfClass:[NSString class]] || !title.length) {
            title = [bundle objectForInfoDictionaryKey:@"CFBundleName"];
        }
        if (![title isKindOfClass:[NSString class]] || !title.length) {
            title = [[location lastPathComponent] stringByDeletingPathExtension];
        }
        *name = strdup([title UTF8String]);

        const size_t size = 32;
        NSImage *icon = [[NSWorkspace sharedWorkspace] iconForFile:location];
        NSRect proposed = NSMakeRect(0, 0, size, size);
        CGImageRef source = [icon CGImageForProposedRect:&proposed context:nil hints:nil];
        if (!source) {
            return;
        }
        CGColorSpaceRef color = CGColorSpaceCreateDeviceRGB();
        CGContextRef context = CGBitmapContextCreate(NULL, size, size, 8, size * 4, color, (CGBitmapInfo)kCGImageAlphaPremultipliedLast);
        CGColorSpaceRelease(color);
        if (!context) {
            return;
        }
        CGContextSetInterpolationQuality(context, kCGInterpolationHigh);
        CGContextDrawImage(context, CGRectMake(0, 0, size, size), source);
        CGImageRef image = CGBitmapContextCreateImage(context);
        if (image) {
            NSBitmapImageRep *bitmap = [[[NSBitmapImageRep alloc] initWithCGImage:image] autorelease];
            NSData *data = [bitmap representationUsingType:NSBitmapImageFileTypePNG properties:@{}];
            if (data.length) {
                *png = strdup([[data base64EncodedStringWithOptions:0] UTF8String]);
            }
            CGImageRelease(image);
        }
        CGContextRelease(context);
    }
}
