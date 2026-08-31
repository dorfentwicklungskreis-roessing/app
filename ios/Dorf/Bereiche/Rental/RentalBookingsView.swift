import SwiftUI

/// „Meine Buchungen" — what I asked for, what was confirmed, and where to
/// pick it up once it was.
struct RentalBookingsView: View {
    @EnvironmentObject private var umgebung: AppUmgebung
    @State private var model: RentalBookingsModel?

    var body: some View {
        Group {
            if let model {
                RentalBookingsList(model: model)
            } else {
                ProgressView().controlSize(.large)
                    .frame(maxWidth: .infinity, maxHeight: .infinity)
            }
        }
        .navigationTitle("Meine Buchungen")
        .navigationBarTitleDisplayMode(.inline)
        .task {
            let existing = model ?? RentalBookingsModel(source: .from(umgebung.rental))
            model = existing
            await existing.load()
        }
    }
}

struct RentalBookingsList: View {
    @ObservedObject var model: RentalBookingsModel
    /// The booking somebody is about to withdraw. Nobody cancels by accident.
    @State private var aboutToCancel: RentalBooking?

    var body: some View {
        List {
            if let trouble = model.trouble, trouble.wantsSignIn {
                Section {
                    // The one case where „try again" would be a lie: the
                    // token is the problem, and a sign-in is the fix.
                    RentalSignInNotice(trouble: trouble) {
                        model.clearTrouble()
                        await model.refresh()
                    }
                }
            } else if let hint = model.hint {
                Section {
                    RentalNotice(text: hint) {
                        Task { await model.refresh() }
                    }
                }
            }

            Section {
                if model.loading && model.bookings.isEmpty {
                    HStack(spacing: 10) {
                        ProgressView()
                        Text("Buchungen werden geholt …").foregroundStyle(.secondary)
                    }
                    .accessibilityIdentifier("rental-bookings-loading")
                }

                ForEach(model.bookings) { booking in
                    VStack(alignment: .leading, spacing: 8) {
                        RentalBookingRow(booking: booking)

                        // Withdrawing is offered because the platform says it
                        // is allowed — the app does not work that out.
                        if booking.canCancel {
                            Button(role: .destructive) {
                                aboutToCancel = booking
                            } label: {
                                HStack(spacing: 8) {
                                    if model.cancelling.contains(booking.id) {
                                        ProgressView().controlSize(.small)
                                    }
                                    Text("Buchung zurückziehen")
                                }
                            }
                            .buttonStyle(.borderless)
                            .disabled(model.cancelling.contains(booking.id))
                            .accessibilityIdentifier("rental-cancel-\(booking.id)")
                        }
                    }
                    .padding(.vertical, 2)
                }

                if model.empty {
                    Text("Du hast noch nichts gebucht. Such dir im Maschinchenring "
                        + "ein Gerät und frag es für deinen Zeitraum an.")
                        .foregroundStyle(.secondary)
                        .accessibilityIdentifier("rental-bookings-empty")
                }
            } footer: {
                Text("Ob eine Anfrage angenommen wird, entscheidet, wem das Gerät "
                    + "gehört. Die Abholadresse steht hier, sobald zugesagt wurde.")
            }
        }
        .accessibilityIdentifier("rental-bookings")
        .refreshable { await model.refresh() }
        .alert(
            "Buchung zurückziehen?",
            isPresented: Binding(
                get: { aboutToCancel != nil },
                set: { if !$0 { aboutToCancel = nil } }
            ),
            presenting: aboutToCancel
        ) { booking in
            Button("Zurückziehen", role: .destructive) {
                Task { await model.cancel(booking) }
            }
            Button("Behalten", role: .cancel) {}
        } message: { booking in
            Text("\(booking.deviceName), \(booking.periodText)")
        }
    }
}

/// One booking, in the list and after it was made.
struct RentalBookingRow: View {
    let booking: RentalBooking

    var body: some View {
        VStack(alignment: .leading, spacing: 4) {
            Text(booking.deviceName)
                .font(.headline)

            Label(booking.periodText, systemImage: "calendar")
                .font(.subheadline)
                .foregroundStyle(.secondary)

            if let back = booking.returnText {
                Text(back)
                    .font(.footnote)
                    .foregroundStyle(.secondary)
            }

            Label(booking.statusLabel, systemImage: booking.state.symbol)
                .font(.subheadline.weight(.medium))
                .foregroundStyle(colour)

            // Personal data only ever appears once the platform hands it
            // over, which it does for one's own confirmed bookings and
            // nowhere else. It is not stored beyond this screen.
            if let address = booking.pickupAddress {
                Label(address, systemImage: "mappin.and.ellipse")
                    .font(.subheadline)
            }
            if let phone = booking.pickupPhone {
                Label(phone, systemImage: "phone")
                    .font(.footnote)
                    .foregroundStyle(.secondary)
            }
            if let note = booking.notes {
                Text(note)
                    .font(.footnote)
                    .foregroundStyle(.secondary)
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .accessibilityElement(children: .combine)
        .accessibilityIdentifier("rental-booking-\(booking.id)")
    }

    /// Colour follows the state, and nothing else does.
    private var colour: Color {
        switch booking.state {
        case .approved: return .green
        case .rejected, .cancelled: return .secondary
        case .pending, .other: return .primary
        }
    }
}
