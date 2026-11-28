//go:build darwin && cgo

#import <Foundation/Foundation.h>
#import <CoreGraphics/CoreGraphics.h>
#import <ImageIO/ImageIO.h>
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

static NSURL *bundleIconURL(NSBundle *bundle) {
    for (NSString *key in @[@"CFBundleIconFile", @"CFBundleIconName"]) {
        id file = [bundle objectForInfoDictionaryKey:key];
        if (![file isKindOfClass:[NSString class]] || ![file length]) {
            continue;
        }
        NSString *extension = [file pathExtension];
        NSURL *url = extension.length
            ? [bundle URLForResource:file withExtension:nil]
            : [bundle URLForResource:file withExtension:@"icns"];
        if (url) {
            return url;
        }
    }
    return nil;
}

static CGImageRef bundleIconImage(NSURL *url, const size_t size) {
    if (!url) {
        return NULL;
    }
    CGImageSourceRef source = CGImageSourceCreateWithURL((CFURLRef)url, NULL);
    if (!source) {
        return NULL;
    }
    // Prefer the smallest representation that will fill the requested size.
    size_t bestIndex = 0;
    size_t bestDistance = SIZE_MAX;
    for (size_t index = 0; index < CGImageSourceGetCount(source); index++) {
        NSDictionary *properties = (NSDictionary *)CGImageSourceCopyPropertiesAtIndex(source, index, NULL);
        size_t width = [properties[(NSString *)kCGImagePropertyPixelWidth] unsignedLongValue];
        size_t height = [properties[(NSString *)kCGImagePropertyPixelHeight] unsignedLongValue];
        [properties release];
        size_t dimension = MIN(width, height);
        size_t distance = dimension >= size ? dimension - size : SIZE_MAX - dimension;
        if (distance < bestDistance) {
            bestIndex = index;
            bestDistance = distance;
        }
    }
    NSDictionary *options = @{
        (NSString *)kCGImageSourceCreateThumbnailFromImageAlways: @YES,
        (NSString *)kCGImageSourceThumbnailMaxPixelSize: @(size),
        (NSString *)kCGImageSourceCreateThumbnailWithTransform: @YES,
    };
    CGImageRef image = CGImageSourceCreateThumbnailAtIndex(source, bestIndex, (CFDictionaryRef)options);
    CFRelease(source);
    return image;
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

        // Decode the bundle resource without opening a connection to the desktop workspace.
        CGImageRef image = bundleIconImage(bundleIconURL(bundle), 32);
        if (!image) {
            return;
        }
        NSMutableData *data = [NSMutableData data];
        CGImageDestinationRef destination = CGImageDestinationCreateWithData((CFMutableDataRef)data, CFSTR("public.png"), 1, NULL);
        if (destination) {
            CGImageDestinationAddImage(destination, image, NULL);
            if (CGImageDestinationFinalize(destination)) {
                *png = strdup([[data base64EncodedStringWithOptions:0] UTF8String]);
            }
            CFRelease(destination);
        }
        CGImageRelease(image);
    }
}
