//go:build darwin

// Native menu bar extra. Kept out of tray_darwin.go because a cgo file with
// //export may only declare, not define, C/Objective-C symbols.

#import <Cocoa/Cocoa.h>

extern void cdmTrayClicked(int tag);

@interface CDMTrayTarget : NSObject
- (void)clicked:(id)sender;
@end

@implementation CDMTrayTarget
- (void)clicked:(id)sender { cdmTrayClicked((int)[sender tag]); }
@end

static NSStatusItem *cdmItem;
static NSMenuItem *cdmStatusLine;
static CDMTrayTarget *cdmTarget;

static void cdmAdd(NSMenu *menu, NSString *title, int tag) {
	NSMenuItem *it = [menu addItemWithTitle:title action:@selector(clicked:) keyEquivalent:@""];
	[it setTarget:cdmTarget];
	[it setTag:tag];
}

void cdmTrayStart(const void *png, int len) {
	NSData *data = [NSData dataWithBytes:png length:len];
	dispatch_async(dispatch_get_main_queue(), ^{
		cdmTarget = [[CDMTrayTarget alloc] init];
		cdmItem = [[[NSStatusBar systemStatusBar] statusItemWithLength:NSSquareStatusItemLength] retain];
		NSImage *img = [[NSImage alloc] initWithData:data];
		[img setSize:NSMakeSize(18, 18)];
		[img setTemplate:YES];
		cdmItem.button.image = img;
		cdmItem.button.toolTip = @"Crazy Download Manager";

		NSMenu *menu = [[NSMenu alloc] init];
		[menu setAutoenablesItems:NO];
		cdmStatusLine = [[menu addItemWithTitle:@"No active downloads" action:nil keyEquivalent:@""] retain];
		[cdmStatusLine setEnabled:NO];
		[menu addItem:[NSMenuItem separatorItem]];
		cdmAdd(menu, @"Show Crazy Download Manager", 1);
		[menu addItem:[NSMenuItem separatorItem]];
		cdmAdd(menu, @"Resume All", 2);
		cdmAdd(menu, @"Pause All", 3);
		[menu addItem:[NSMenuItem separatorItem]];
		cdmAdd(menu, @"Quit Crazy Download Manager", 4);
		cdmItem.menu = menu;
	});
}

void cdmTraySetStatus(const char *s) {
	NSString *text = [NSString stringWithUTF8String:s];
	dispatch_async(dispatch_get_main_queue(), ^{
		if (cdmItem == nil) return;
		[cdmStatusLine setTitle:text];
		cdmItem.button.toolTip = [@"Crazy Download Manager\n" stringByAppendingString:text];
	});
}

void cdmTrayStop(void) {
	dispatch_async(dispatch_get_main_queue(), ^{
		if (cdmItem == nil) return;
		[[NSStatusBar systemStatusBar] removeStatusItem:cdmItem];
		cdmItem = nil;
	});
}
